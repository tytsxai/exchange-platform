// Package ws publishes balance events to Redis.
package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

const privateUserEventChannelTemplate = "private:user:{userId}:events"

// Publisher publishes balance events.
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

// BalanceDelta represents a balance change payload for freeze/unfreeze.
type BalanceDelta struct {
	Asset  string `json:"asset"`
	Amount int64  `json:"amount"`
}

// PublishFrozenEvent publishes a frozen event for the user.
func (p *Publisher) PublishFrozenEvent(ctx context.Context, userID int64, asset string, amount int64) error {
	return p.publish(ctx, userID, "frozen", BalanceDelta{
		Asset:  asset,
		Amount: amount,
	})
}

// PublishUnfrozenEvent publishes an unfrozen event for the user.
func (p *Publisher) PublishUnfrozenEvent(ctx context.Context, userID int64, asset string, amount int64) error {
	return p.publish(ctx, userID, "unfrozen", BalanceDelta{
		Asset:  asset,
		Amount: amount,
	})
}

// PublishSettledEvent publishes a settled event for the user.
func (p *Publisher) PublishSettledEvent(ctx context.Context, userID int64, data interface{}) error {
	return p.publish(ctx, userID, "settled", data)
}

func (p *Publisher) publish(ctx context.Context, userID int64, event string, data interface{}) error {
	payload := map[string]interface{}{
		"channel": "balance",
		"event":   event,
		"data":    data,
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
