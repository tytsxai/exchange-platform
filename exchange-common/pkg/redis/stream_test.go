package redis

import (
	"context"
	"testing"

	goredis "github.com/redis/go-redis/v9"
)

func TestNewConsumerDefaultsPendingInterval(t *testing.T) {
	client := NewStreamClient(goredis.NewClient(&goredis.Options{Addr: "localhost:6379"}))
	opts := &ConsumerOptions{BatchSize: 5}

	consumer := NewConsumer(client, "group", "consumer", []string{"stream"}, func(ctx context.Context, msg *Message) error {
		return nil
	}, opts)

	if consumer.opts.PendingCheckInterval != DefaultConsumerOptions.PendingCheckInterval {
		t.Fatalf("PendingCheckInterval = %v, want %v", consumer.opts.PendingCheckInterval, DefaultConsumerOptions.PendingCheckInterval)
	}
	if consumer.opts.BatchSize != 5 {
		t.Fatalf("BatchSize = %d, want 5", consumer.opts.BatchSize)
	}
}
