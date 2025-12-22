// Package ws Redis pub/sub consumer for private events.
package ws

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

const privateUserEventChannelTemplate = "private:user:{userId}:events"

// Consumer listens for private events and broadcasts to users.
type Consumer struct {
	client          *redis.Client
	hub             *Hub
	channelTemplate string
}

// NewConsumer creates a new consumer.
func NewConsumer(client *redis.Client, hub *Hub, channel string) *Consumer {
	if channel == "" {
		channel = privateUserEventChannelTemplate
	}
	return &Consumer{
		client:          client,
		hub:             hub,
		channelTemplate: channel,
	}
}

// Run starts the pub/sub loop.
func (c *Consumer) Run(ctx context.Context) error {
	pattern, hasUserID := toUserPattern(c.channelTemplate)
	var pubsub *redis.PubSub
	if hasUserID {
		pubsub = c.client.PSubscribe(ctx, pattern)
	} else {
		pubsub = c.client.Subscribe(ctx, c.channelTemplate)
	}
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			c.handleMessage(msg.Channel, msg.Payload)
		}
	}
}

type privateEvent struct {
	UserID  int64           `json:"user_id"`
	Channel string          `json:"channel"`
	Event   string          `json:"event"`
	Data    json.RawMessage `json:"data"`
}

func (c *Consumer) handleMessage(channel string, payload string) {
	var event privateEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		log.Printf("private event decode error: %v", err)
		return
	}

	userID := event.UserID
	if userID == 0 {
		if parsed, ok := parseUserID(c.channelTemplate, channel); ok {
			userID = parsed
		}
	}
	if userID == 0 {
		log.Printf("private event missing user id")
		return
	}

	message, err := json.Marshal(PrivateMessage{
		Channel: event.Channel,
		Event:   event.Event,
		Data:    event.Data,
	})
	if err != nil {
		log.Printf("private event encode error: %v", err)
		return
	}

	c.hub.Broadcast(userID, message)
}

func toUserPattern(template string) (string, bool) {
	parts := strings.Split(template, "{userId}")
	if len(parts) != 2 {
		return template, false
	}
	return parts[0] + "*" + parts[1], true
}

func parseUserID(template string, channel string) (int64, bool) {
	parts := strings.Split(template, "{userId}")
	if len(parts) != 2 {
		return 0, false
	}
	if !strings.HasPrefix(channel, parts[0]) || !strings.HasSuffix(channel, parts[1]) {
		return 0, false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(channel, parts[0]), parts[1])
	if raw == "" {
		return 0, false
	}
	userID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return userID, true
}
