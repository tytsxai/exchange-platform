package ws

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

func TestAuthConsumerRun(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	hub := NewHub()
	conn := &websocket.Conn{}
	clientConn, err := hub.Subscribe(11, conn)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	consumer := NewConsumer(client, hub, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- consumer.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	payload := map[string]interface{}{
		"channel": "order",
		"data": map[string]interface{}{
			"id": 1,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := client.Publish(ctx, "private:user:11:events", raw).Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case <-clientConn.send:
	case <-time.After(1 * time.Second):
		t.Fatal("expected broadcast message")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("consumer did not stop")
	}
}
