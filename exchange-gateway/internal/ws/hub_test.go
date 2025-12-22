package ws

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestAuthHubSubscribeBroadcastUnsubscribe(t *testing.T) {
	hub := NewHub()
	conn1 := &websocket.Conn{}
	conn2 := &websocket.Conn{}

	client1, err := hub.Subscribe(1, conn1)
	if err != nil {
		t.Fatalf("subscribe client1: %v", err)
	}
	client2, err := hub.Subscribe(1, conn2)
	if err != nil {
		t.Fatalf("subscribe client2: %v", err)
	}

	message := []byte("hello")
	hub.Broadcast(1, message)

	select {
	case got := <-client1.send:
		if string(got) != string(message) {
			t.Fatalf("client1 message = %s, want %s", string(got), string(message))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("client1 did not receive message")
	}

	select {
	case got := <-client2.send:
		if string(got) != string(message) {
			t.Fatalf("client2 message = %s, want %s", string(got), string(message))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("client2 did not receive message")
	}

	hub.Unsubscribe(1, client1)
	select {
	case _, ok := <-client1.send:
		if ok {
			t.Fatal("client1 send channel should be closed")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("client1 send channel not closed")
	}
}

func TestAuthHubMaxConnectionsAndStats(t *testing.T) {
	hub := NewHubWithMaxConnections(1)
	conn1 := &websocket.Conn{}
	conn2 := &websocket.Conn{}

	client1, err := hub.Subscribe(5, conn1)
	if err != nil {
		t.Fatalf("subscribe client1: %v", err)
	}

	if _, err := hub.Subscribe(5, conn2); err == nil {
		t.Fatal("expected max connections error")
	}

	stats := hub.Stats()
	if stats.ActiveConnections != 1 {
		t.Fatalf("active connections = %d, want 1", stats.ActiveConnections)
	}
	if stats.TotalConnections != 1 {
		t.Fatalf("total connections = %d, want 1", stats.TotalConnections)
	}
	if stats.Users != 1 {
		t.Fatalf("users = %d, want 1", stats.Users)
	}

	hub.Unsubscribe(5, client1)
	stats = hub.Stats()
	if stats.ActiveConnections != 0 {
		t.Fatalf("active connections = %d, want 0", stats.ActiveConnections)
	}
	if stats.Users != 0 {
		t.Fatalf("users = %d, want 0", stats.Users)
	}
}

func TestAuthHubConnectionCountAndLastActive(t *testing.T) {
	hub := NewHubWithMaxConnections(0)
	conn := &websocket.Conn{}

	client, err := hub.Subscribe(9, conn)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if hub.ConnectionCount() != 1 {
		t.Fatalf("connection count = %d, want 1", hub.ConnectionCount())
	}
	if client.lastActive().IsZero() {
		t.Fatal("expected last activity timestamp")
	}

	hub.Unsubscribe(9, client)
	if hub.ConnectionCount() != 0 {
		t.Fatalf("connection count = %d, want 0", hub.ConnectionCount())
	}
}
