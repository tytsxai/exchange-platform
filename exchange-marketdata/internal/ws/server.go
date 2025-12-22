// Package ws WebSocket 服务
package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/exchange/marketdata/internal/service"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源
	},
}

// Server WebSocket 服务器
type Server struct {
	svc     *service.MarketDataService
	clients map[*Client]bool
	mu      sync.RWMutex
}

// Client WebSocket 客户端
type Client struct {
	conn          *websocket.Conn
	server        *Server
	svc           *service.MarketDataService
	subscriptions map[string]chan *service.Event
	send          chan []byte
	mu            sync.Mutex
}

// NewServer 创建 WebSocket 服务器
func NewServer(svc *service.MarketDataService) *Server {
	return &Server{
		svc:     svc,
		clients: make(map[*Client]bool),
	}
}

// HandleWS 处理 WebSocket 连接
func (s *Server) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
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
		c.server.removeClient(c)
		c.conn.Close()
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
		c.conn.Close()
	}()

	for {
		select {
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
			select {
			case c.send <- data:
			default:
				// 发送队列满
			}
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
	select {
	case c.send <- data:
	default:
	}
}

func (c *Client) sendError(msg string) {
	c.sendResponse(&WsResponse{Error: msg})
}

func (s *Server) removeClient(c *Client) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.clients[c]; ok {
		delete(s.clients, c)

		// 取消所有订阅
		c.mu.Lock()
		for channel, ch := range c.subscriptions {
			c.svc.Unsubscribe(channel, ch)
		}
		c.mu.Unlock()

		close(c.send)
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

// Run 运行 WebSocket 服务器
func (s *Server) Run(ctx context.Context, addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.HandleWS)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	log.Printf("WebSocket server listening on %s", addr)
	return server.ListenAndServe()
}
