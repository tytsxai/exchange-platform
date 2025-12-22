package ws

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestAuthConsumerHandleMessage(t *testing.T) {
	hub := NewHub()
	conn := &websocket.Conn{}
	client, err := hub.Subscribe(7, conn)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	consumer := &Consumer{hub: hub}

	payload := map[string]interface{}{
		"user_id": 7,
		"channel": "trade",
		"data": map[string]interface{}{
			"id": 55,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	consumer.handleMessage("private:user:7:events", string(raw))

	select {
	case got := <-client.send:
		var msg PrivateMessage
		if err := json.Unmarshal(got, &msg); err != nil {
			t.Fatalf("unmarshal message: %v", err)
		}
		if msg.Channel != "trade" {
			t.Fatalf("channel = %s, want trade", msg.Channel)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected message broadcast")
	}
}

func TestAuthConsumerHandleMessageInvalidJSON(t *testing.T) {
	hub := NewHub()
	conn := &websocket.Conn{}
	client, err := hub.Subscribe(1, conn)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	consumer := &Consumer{hub: hub}
	consumer.handleMessage("private:user:1:events", "{not-json")

	select {
	case <-client.send:
		t.Fatal("unexpected message broadcast")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestAuthConsumerHandleMessageParsesUserIDFromChannel(t *testing.T) {
	hub := NewHub()
	conn := &websocket.Conn{}
	client, err := hub.Subscribe(42, conn)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	consumer := &Consumer{hub: hub, channelTemplate: privateUserEventChannelTemplate}

	payload := map[string]interface{}{
		"user_id": 0,
		"channel": "balance",
		"data":    map[string]interface{}{"id": 7},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	consumer.handleMessage("private:user:42:events", string(raw))

	select {
	case <-client.send:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected message broadcast from channel user id")
	}
}

func TestAuthConsumerHandleMessageMissingUserIDFromChannel(t *testing.T) {
	hub := NewHub()
	conn := &websocket.Conn{}
	client, err := hub.Subscribe(1, conn)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	consumer := &Consumer{hub: hub, channelTemplate: privateUserEventChannelTemplate}
	payload := map[string]interface{}{
		"user_id": 0,
		"channel": "trade",
		"data":    map[string]interface{}{"id": 3},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	consumer.handleMessage("private:user::events", string(raw))

	select {
	case <-client.send:
		t.Fatal("unexpected message broadcast")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestAuthToUserPatternAndParseUserID(t *testing.T) {
	pattern, ok := toUserPattern(privateUserEventChannelTemplate)
	if !ok {
		t.Fatal("expected pattern match")
	}
	if pattern != "private:user:*:events" {
		t.Fatalf("pattern = %s", pattern)
	}

	if _, ok := toUserPattern("private:events"); ok {
		t.Fatal("expected no placeholder match")
	}

	if _, ok := parseUserID("private:events", "private:events"); ok {
		t.Fatal("expected parse failure for missing placeholder")
	}
	if _, ok := parseUserID(privateUserEventChannelTemplate, "private:user:1:events:extra"); ok {
		t.Fatal("expected parse failure for suffix mismatch")
	}
	if _, ok := parseUserID(privateUserEventChannelTemplate, "private:user::events"); ok {
		t.Fatal("expected parse failure for empty user id")
	}
	if _, ok := parseUserID(privateUserEventChannelTemplate, "private:user:notnum:events"); ok {
		t.Fatal("expected parse failure for non-numeric user id")
	}
}

func TestAuthConsumerHandleMessageMissingUserIDNoTemplate(t *testing.T) {
	hub := NewHub()
	conn := &websocket.Conn{}
	client, err := hub.Subscribe(3, conn)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	consumer := &Consumer{hub: hub, channelTemplate: "private:events"}

	payload := map[string]interface{}{
		"channel": "balance",
		"data": map[string]interface{}{
			"id": 9,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	consumer.handleMessage("private:events", string(raw))

	select {
	case <-client.send:
		t.Fatal("unexpected message broadcast")
	case <-time.After(100 * time.Millisecond):
	}
}
