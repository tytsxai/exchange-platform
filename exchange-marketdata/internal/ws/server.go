// Package ws WebSocket 服务
package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/exchange/marketdata/internal/service"
	"github.com/gorilla/websocket"
)

type Config struct {
	AllowedOrigins          []string
	MaxSubscriptionsPerConn int
}

// Server WebSocket 服务器
type Server struct {
	svc     *service.MarketDataService
	clients map[*Client]bool
	mu      sync.RWMutex

	upgrader websocket.Upgrader
	cfg      Config
}

// Client WebSocket 客户端
type Client struct {
	conn          *websocket.Conn
	server        *Server
	svc           *service.MarketDataService
	subscriptions map[string]chan *service.Event
	send          chan []byte
	mu            sync.Mutex
	closed        chan struct{}
	closeOnce     sync.Once
}

// NewServer 创建 WebSocket 服务器
func NewServer(svc *service.MarketDataService, cfg *Config) *Server {
	c := Config{
		AllowedOrigins:          nil,
		MaxSubscriptionsPerConn: 50,
	}
	if cfg != nil {
		if cfg.AllowedOrigins != nil {
			c.AllowedOrigins = cfg.AllowedOrigins
		}
		if cfg.MaxSubscriptionsPerConn > 0 {
			c.MaxSubscriptionsPerConn = cfg.MaxSubscriptionsPerConn
		}
	}

	s := &Server{
		svc:     svc,
		clients: make(map[*Client]bool),
		cfg:     c,
	}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return allowOrigin(r, s.cfg.AllowedOrigins)
		},
	}
	return s
}

// HandleWS 处理 WebSocket 连接
func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	client := &Client{
		conn:          conn,
		server:        s,
		svc:           s.svc,
		subscriptions: make(map[string]chan *service.Event),
		send:          make(chan []byte, 256),
		closed:        make(chan struct{}),
	}

	s.mu.Lock()
	s.clients[client] = true
	s.mu.Unlock()

	go client.writePump()
	go client.readPump()
}

// WsRequest WebSocket 请求
type WsRequest struct {
	Op      string `json:"op"`
	Channel string `json:"channel"`
}

// WsResponse WebSocket 响应
type WsResponse struct {
	Op      string      `json:"op,omitempty"`
	Channel string      `json:"channel,omitempty"`
	Success bool        `json:"success,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func (c *Client) readPump() {
	defer func() {
		c.close()
		c.server.removeClient(c)
	}()

	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Read error: %v", err)
			}
			break
		}

		var req WsRequest
		if err := json.Unmarshal(message, &req); err != nil {
			c.sendError("invalid request")
			continue
		}

		c.handleRequest(&req)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case <-c.closed:
			return
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleRequest(req *WsRequest) {
	switch req.Op {
	case "subscribe":
		c.subscribe(req.Channel)
	case "unsubscribe":
		c.unsubscribe(req.Channel)
	case "ping":
		c.sendResponse(&WsResponse{Op: "pong"})
	default:
		c.sendError("unknown op")
	}
}

func (c *Client) subscribe(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if channel == "" {
		c.sendError("channel required")
		return
	}
	if len(channel) > 128 {
		c.sendError("channel too long")
		return
	}
	if _, err := validateChannel(channel); err != nil {
		c.sendError(err.Error())
		return
	}
	if max := c.server.cfg.MaxSubscriptionsPerConn; max > 0 && len(c.subscriptions) >= max {
		c.sendError("too many subscriptions")
		return
	}

	if _, exists := c.subscriptions[channel]; exists {
		c.sendResponse(&WsResponse{Op: "subscribe", Channel: channel, Success: true})
		return
	}

	ch := c.svc.Subscribe(channel)
	c.subscriptions[channel] = ch

	// 启动转发 goroutine
	go func() {
		for event := range ch {
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			c.trySend(data)
		}
	}()

	c.sendResponse(&WsResponse{Op: "subscribe", Channel: channel, Success: true})
}

func (c *Client) unsubscribe(channel string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ch, exists := c.subscriptions[channel]
	if !exists {
		c.sendResponse(&WsResponse{Op: "unsubscribe", Channel: channel, Success: true})
		return
	}

	c.svc.Unsubscribe(channel, ch)
	delete(c.subscriptions, channel)

	c.sendResponse(&WsResponse{Op: "unsubscribe", Channel: channel, Success: true})
}

func (c *Client) sendResponse(resp *WsResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	c.trySend(data)
}

func (c *Client) sendError(msg string) {
	c.sendResponse(&WsResponse{Error: msg})
}

func (c *Client) trySend(data []byte) {
	select {
	case <-c.closed:
		return
	default:
	}
	select {
	case c.send <- data:
	default:
	}
}

func (c *Client) close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.conn.Close()
	})
}

func (s *Server) removeClient(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.clients[c]; ok {
		delete(s.clients, c)

		c.close()

		// 取消所有订阅
		c.mu.Lock()
		for channel, ch := range c.subscriptions {
			c.svc.Unsubscribe(channel, ch)
		}
		c.mu.Unlock()

	}
}

// Broadcast 广播消息
func (s *Server) Broadcast(message []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for client := range s.clients {
		select {
		case client.send <- message:
		default:
		}
	}
}

// ClientCount 客户端数量
func (s *Server) ClientCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

func (s *Server) CloseAll() {
	s.mu.RLock()
	conns := make([]*websocket.Conn, 0, len(s.clients))
	for c := range s.clients {
		conns = append(conns, c.conn)
	}
	s.mu.RUnlock()

	for _, conn := range conns {
		_ = conn.Close()
	}
}

// Run 运行 WebSocket 服务器
func (s *Server) Run(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.HandleWS)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.CloseAll()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("WebSocket server listening on %s", addr)
	return server.ListenAndServe()
}

func allowOrigin(r *http.Request, allowed []string) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		// Non-browser clients usually don't send Origin.
		return true
	}
	for _, o := range allowed {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o == "*" || o == origin {
			return true
		}
	}
	return false
}

func validateChannel(channel string) (string, error) {
	// Expected: market.<SYMBOL>.(book|trades|ticker)
	parts := strings.Split(channel, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid channel")
	}
	if parts[0] != "market" {
		return "", fmt.Errorf("invalid channel")
	}
	symbol := parts[1]
	if len(symbol) < 1 || len(symbol) > 32 {
		return "", fmt.Errorf("invalid symbol")
	}
	for i := 0; i < len(symbol); i++ {
		b := symbol[i]
		if !(b >= 'A' && b <= 'Z') && !(b >= '0' && b <= '9') {
			return "", fmt.Errorf("invalid symbol")
		}
	}
	switch parts[2] {
	case "book", "trades", "ticker":
		return channel, nil
	default:
		return "", fmt.Errorf("invalid channel")
	}
}
