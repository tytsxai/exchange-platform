package ws

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestPublisherPublishOrderEvent(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	publisher := NewPublisher(client, privateUserEventChannelTemplate)

	testCases := []struct {
		name  string
		event string
		send  func(ctx context.Context) error
	}{
		{
			name:  "created",
			event: "created",
			send: func(ctx context.Context) error {
				return publisher.PublishOrderCreated(ctx, 42, map[string]interface{}{
					"id":     1001,
					"symbol": "BTC-USDT",
				})
			},
		},
		{
			name:  "filled",
			event: "filled",
			send: func(ctx context.Context) error {
				return publisher.PublishOrderFilled(ctx, 42, map[string]interface{}{
					"id":     1002,
					"symbol": "BTC-USDT",
				})
			},
		},
		{
			name:  "canceled",
			event: "canceled",
			send: func(ctx context.Context) error {
				return publisher.PublishOrderCanceled(ctx, 42, map[string]interface{}{
					"id":     1003,
					"symbol": "BTC-USDT",
				})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			sub := client.Subscribe(ctx, "private:user:42:events")
			defer sub.Close()
			if _, err := sub.Receive(ctx); err != nil {
				t.Fatalf("subscribe: %v", err)
			}

			if err := tc.send(ctx); err != nil {
				t.Fatalf("publish: %v", err)
			}

			msg, err := sub.ReceiveMessage(ctx)
			if err != nil {
				t.Fatalf("receive: %v", err)
			}

			var payload map[string]interface{}
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if payload["channel"].(string) != "order" {
				t.Fatalf("channel = %v, want order", payload["channel"])
			}
			if payload["event"].(string) != tc.event {
				t.Fatalf("event = %v, want %s", payload["event"], tc.event)
			}
		})
	}
}

func TestPublisherPublishTradeEvent(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	publisher := NewPublisher(client, "")
	format, hasUserID := normalizeUserChannelFormat(privateUserEventChannelTemplate)
	if !hasUserID {
		t.Fatal("expected template to include userId placeholder")
	}
	if publisher.channelFormat != format {
		t.Fatalf("channel = %s, want %s", publisher.channelFormat, format)
	}
	if !publisher.hasUserID {
		t.Fatal("expected publisher to format userId")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sub := client.Subscribe(ctx, "private:user:99:events")
	defer sub.Close()
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	trade := map[string]interface{}{
		"id":     2001,
		"symbol": "ETH-USDT",
	}

	if err := publisher.PublishTradeEvent(ctx, 99, trade); err != nil {
		t.Fatalf("publish: %v", err)
	}

	msg, err := sub.ReceiveMessage(ctx)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if payload["channel"].(string) != "trade" {
		t.Fatalf("channel = %v, want trade", payload["channel"])
	}
	if _, ok := payload["event"]; ok {
		t.Fatalf("event should be omitted for trade payload")
	}
}
