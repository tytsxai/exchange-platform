// Package ws Redis pub/sub consumer for private events.
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/exchange/common/pkg/health"
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
	return c.run(ctx, nil)
}

// RunWithMonitor starts the pub/sub loop and updates monitor periodically.
// It is used for production readiness checks (consumer loop liveness).
func (c *Consumer) RunWithMonitor(ctx context.Context, monitor *health.LoopMonitor) error {
	return c.run(ctx, monitor)
}

func (c *Consumer) run(ctx context.Context, monitor *health.LoopMonitor) error {
	pattern, hasUserID := toUserPattern(c.channelTemplate)
	var pubsub *redis.PubSub
	if hasUserID {
		pubsub = c.client.PSubscribe(ctx, pattern)
	} else {
		pubsub = c.client.Subscribe(ctx, c.channelTemplate)
	}
	defer pubsub.Close()

	ch := pubsub.Channel()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	if monitor != nil {
		monitor.Tick()
	}

	for {
		select {
		case <-ctx.Done():
			if monitor != nil {
				monitor.SetError(ctx.Err())
			}
			return ctx.Err()
		case <-ticker.C:
			if monitor != nil {
				monitor.Tick()
			}
		case msg, ok := <-ch:
			if !ok {
				if monitor != nil {
					monitor.SetError(errors.New("pubsub channel closed"))
				}
				return nil
			}
			if monitor != nil {
				monitor.Tick()
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
