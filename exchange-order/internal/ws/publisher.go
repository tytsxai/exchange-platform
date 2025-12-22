// Package ws publishes private events to Redis.
package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

const privateUserEventChannelTemplate = "private:user:{userId}:events"

// Publisher publishes private events.
type Publisher struct {
	client        *redis.Client
	channelFormat string
	hasUserID     bool
}

// NewPublisher creates a publisher.
func NewPublisher(client *redis.Client, channel string) *Publisher {
	if channel == "" {
		channel = privateUserEventChannelTemplate
	}
	format, hasUserID := normalizeUserChannelFormat(channel)
	return &Publisher{
		client:        client,
		channelFormat: format,
		hasUserID:     hasUserID,
	}
}

// PublishOrderEvent publishes an order event for the user.
func (p *Publisher) PublishOrderEvent(ctx context.Context, userID int64, event string, order interface{}) error {
	return p.publish(ctx, userID, "order", event, order)
}

// PublishOrderCreated publishes an order created event for the user.
func (p *Publisher) PublishOrderCreated(ctx context.Context, userID int64, order interface{}) error {
	return p.PublishOrderEvent(ctx, userID, "created", order)
}

// PublishOrderFilled publishes an order filled event for the user.
func (p *Publisher) PublishOrderFilled(ctx context.Context, userID int64, order interface{}) error {
	return p.PublishOrderEvent(ctx, userID, "filled", order)
}

// PublishOrderCanceled publishes an order canceled event for the user.
func (p *Publisher) PublishOrderCanceled(ctx context.Context, userID int64, order interface{}) error {
	return p.PublishOrderEvent(ctx, userID, "canceled", order)
}

// PublishTradeEvent publishes a trade event for the user.
func (p *Publisher) PublishTradeEvent(ctx context.Context, userID int64, trade interface{}) error {
	return p.publish(ctx, userID, "trade", "", trade)
}

func (p *Publisher) publish(ctx context.Context, userID int64, channel string, event string, data interface{}) error {
	payload := map[string]interface{}{
		"channel": channel,
		"data":    data,
	}
	if event != "" {
		payload["event"] = event
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	targetChannel := p.channelFormat
	if p.hasUserID {
		targetChannel = fmt.Sprintf(p.channelFormat, userID)
	}
	return p.client.Publish(ctx, targetChannel, raw).Err()
}

func normalizeUserChannelFormat(template string) (string, bool) {
	if strings.Contains(template, "{userId}") {
		return strings.ReplaceAll(template, "{userId}", "%d"), true
	}
	return template, false
}
