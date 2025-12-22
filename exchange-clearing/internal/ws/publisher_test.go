package ws

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestPublisherPublishFrozenEvent(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	publisher := NewPublisher(client, "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	channel := "private:user:42:events"
	sub := client.Subscribe(ctx, channel)
	defer sub.Close()

	if err := publisher.PublishFrozenEvent(ctx, 42, "BTC", 1500); err != nil {
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

	if payload["channel"].(string) != "balance" {
		t.Fatalf("channel = %v, want balance", payload["channel"])
	}
	if payload["event"].(string) != "frozen" {
		t.Fatalf("event = %v, want frozen", payload["event"])
	}

	data := payload["data"].(map[string]interface{})
	if data["asset"].(string) != "BTC" {
		t.Fatalf("asset = %v, want BTC", data["asset"])
	}
	if data["amount"].(float64) != 1500 {
		t.Fatalf("amount = %v, want 1500", data["amount"])
	}
}

func TestPublisherPublishUnfrozenEvent(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	publisher := NewPublisher(client, "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	channel := "private:user:7:events"
	sub := client.Subscribe(ctx, channel)
	defer sub.Close()

	if err := publisher.PublishUnfrozenEvent(ctx, 7, "USDT", 300); err != nil {
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

	if payload["event"].(string) != "unfrozen" {
		t.Fatalf("event = %v, want unfrozen", payload["event"])
	}
}

func TestPublisherPublishSettledEvent(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	publisher := NewPublisher(client, "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	channel := "private:user:88:events"
	sub := client.Subscribe(ctx, channel)
	defer sub.Close()

	data := map[string]interface{}{
		"asset":  "ETH",
		"amount": 12,
		"trade":  "T-1001",
	}

	if err := publisher.PublishSettledEvent(ctx, 88, data); err != nil {
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

	if payload["event"].(string) != "settled" {
		t.Fatalf("event = %v, want settled", payload["event"])
	}
	if payload["channel"].(string) != "balance" {
		t.Fatalf("channel = %v, want balance", payload["channel"])
	}
}
