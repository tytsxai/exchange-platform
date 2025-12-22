// Package ws WebSocket hub for private events.
package ws

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const defaultMaxConnectionsPerUser = 5

// ErrMaxConnections is returned when a user exceeds the connection limit.
var ErrMaxConnections = errors.New("max connections per user exceeded")

// Client wraps a websocket connection with a send channel.
type Client struct {
	conn         *websocket.Conn
	send         chan []byte
	lastActivity int64
}

func (c *Client) touch() {
	atomic.StoreInt64(&c.lastActivity, time.Now().UnixNano())
}

func (c *Client) lastActive() time.Time {
	return time.Unix(0, atomic.LoadInt64(&c.lastActivity))
}

// Hub manages websocket connections per user.
type Hub struct {
	mu                sync.RWMutex
	conns             map[int64]map[*Client]struct{}
	maxPerUser        int
	total             int64
	activeConnections int64
}

// NewHub creates a new hub.
func NewHub() *Hub {
	return &Hub{
		conns:      make(map[int64]map[*Client]struct{}),
		maxPerUser: defaultMaxConnectionsPerUser,
	}
}

// NewHubWithMaxConnections creates a hub with a custom per-user limit.
func NewHubWithMaxConnections(limit int) *Hub {
	if limit <= 0 {
		limit = defaultMaxConnectionsPerUser
	}
	return &Hub{
		conns:      make(map[int64]map[*Client]struct{}),
		maxPerUser: limit,
	}
}

// Subscribe registers a connection for a user and returns the client wrapper.
func (h *Hub) Subscribe(userID int64, conn *websocket.Conn) (*Client, error) {
	client := &Client{
		conn: conn,
		send: make(chan []byte, 256),
	}
	client.touch()

	h.mu.Lock()
	defer h.mu.Unlock()

	clients, ok := h.conns[userID]
	if !ok {
		clients = make(map[*Client]struct{})
		h.conns[userID] = clients
	}
	if h.maxPerUser > 0 && len(clients) >= h.maxPerUser {
		return nil, ErrMaxConnections
	}
	clients[client] = struct{}{}
	atomic.AddInt64(&h.total, 1)
	atomic.AddInt64(&h.activeConnections, 1)

	return client, nil
}

// Unsubscribe removes a connection for a user.
func (h *Hub) Unsubscribe(userID int64, client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	clients, ok := h.conns[userID]
	if !ok {
		return
	}

	if _, ok := clients[client]; !ok {
		return
	}
	delete(clients, client)
	close(client.send)
	atomic.AddInt64(&h.total, -1)
	atomic.AddInt64(&h.activeConnections, -1)

	if len(clients) == 0 {
		delete(h.conns, userID)
	}
}

// Broadcast sends a message to all connections for the user.
func (h *Hub) Broadcast(userID int64, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	clients, ok := h.conns[userID]
	if !ok {
		return
	}

	for client := range clients {
		select {
		case client.send <- message:
		default:
			// Drop if the client is slow.
		}
	}
}

// ConnectionCount returns total active websocket connections.
func (h *Hub) ConnectionCount() int64 {
	return atomic.LoadInt64(&h.total)
}

// HubStats provides connection metrics.
type HubStats struct {
	ActiveConnections int64
	TotalConnections  int64
	Users             int
	MaxPerUser        int
}

// Stats returns connection metrics for the hub.
func (h *Hub) Stats() HubStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return HubStats{
		ActiveConnections: atomic.LoadInt64(&h.activeConnections),
		TotalConnections:  atomic.LoadInt64(&h.total),
		Users:             len(h.conns),
		MaxPerUser:        h.maxPerUser,
	}
}

// CloseAll closes all active websocket connections.
func (h *Hub) CloseAll() {
	h.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(h.conns))
	for _, userConns := range h.conns {
		for client := range userConns {
			clients = append(clients, client.conn)
		}
	}
	h.mu.RUnlock()

	for _, conn := range clients {
		_ = conn.Close()
	}
}
