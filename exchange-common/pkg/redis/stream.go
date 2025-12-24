// Package redis Redis Streams 封装
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// StreamClient Redis Streams 客户端
type StreamClient struct {
	client *redis.Client
}

// NewStreamClient 创建客户端
func NewStreamClient(client *redis.Client) *StreamClient {
	return &StreamClient{client: client}
}

// Publish 发布消息到 Stream
func (c *StreamClient) Publish(ctx context.Context, stream string, msg interface{}) (string, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("marshal message: %w", err)
	}

	id, err := c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{
			"data": string(data),
		},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("xadd: %w", err)
	}

	return id, nil
}

// PublishWithID 发布消息并指定 ID（用于幂等）
func (c *StreamClient) PublishWithID(ctx context.Context, stream, id string, msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	_, err = c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		ID:     id,
		Values: map[string]interface{}{
			"data": string(data),
		},
	}).Result()
	if err != nil {
		return fmt.Errorf("xadd: %w", err)
	}

	return nil
}

// Message 消息
type Message struct {
	ID     string
	Stream string
	Data   []byte
}

// Consumer 消费者
type Consumer struct {
	client   *StreamClient
	group    string
	consumer string
	streams  []string
	handler  MessageHandler
	opts     ConsumerOptions
}

// MessageHandler 消息处理函数
type MessageHandler func(ctx context.Context, msg *Message) error

// ConsumerOptions 消费者选项
type ConsumerOptions struct {
	BatchSize    int           // 每次读取的消息数
	BlockTime    time.Duration // 阻塞等待时间
	MaxRetries   int           // 最大重试次数
	RetryBackoff time.Duration // 重试间隔
	ClaimMinIdle time.Duration // 认领空闲消息的最小时间
	// PendingCheckInterval 周期性处理 pending 的间隔
	PendingCheckInterval time.Duration
}

// DefaultConsumerOptions 默认选项
var DefaultConsumerOptions = ConsumerOptions{
	BatchSize:            10,
	BlockTime:            5 * time.Second,
	MaxRetries:           3,
	RetryBackoff:         time.Second,
	ClaimMinIdle:         30 * time.Second,
	PendingCheckInterval: 30 * time.Second,
}

// NewConsumer 创建消费者
func NewConsumer(client *StreamClient, group, consumer string, streams []string, handler MessageHandler, opts *ConsumerOptions) *Consumer {
	if opts == nil {
		opts = &DefaultConsumerOptions
	}
	return &Consumer{
		client:   client,
		group:    group,
		consumer: consumer,
		streams:  streams,
		handler:  handler,
		opts:     *opts,
	}
}

// Start 启动消费
func (c *Consumer) Start(ctx context.Context) error {
	// 确保消费者组存在
	for _, stream := range c.streams {
		err := c.client.client.XGroupCreateMkStream(ctx, stream, c.group, "0").Err()
		if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
			return fmt.Errorf("create group: %w", err)
		}
	}

	// 先处理 pending 消息
	if err := c.processPending(ctx); err != nil {
		return fmt.Errorf("process pending: %w", err)
	}

	// 消费新消息
	return c.consume(ctx)
}

// processPending 处理 pending 消息
func (c *Consumer) processPending(ctx context.Context) error {
	for _, stream := range c.streams {
		for {
			// 读取 pending 消息
			pending, err := c.client.client.XPendingExt(ctx, &redis.XPendingExtArgs{
				Stream: stream,
				Group:  c.group,
				Start:  "-",
				End:    "+",
				Count:  int64(c.opts.BatchSize),
			}).Result()
			if err != nil {
				return fmt.Errorf("xpending: %w", err)
			}

			if len(pending) == 0 {
				break
			}

			// 认领并处理（带最大重试/死信）
			ids := make([]string, 0, len(pending))
			dlqIDs := make(map[string]int64)
			for _, p := range pending {
				if p.Idle >= c.opts.ClaimMinIdle {
					ids = append(ids, p.ID)
					if c.opts.MaxRetries > 0 && p.RetryCount > int64(c.opts.MaxRetries) {
						dlqIDs[p.ID] = p.RetryCount
					}
				}
			}

			if len(ids) == 0 {
				break
			}

			messages, err := c.client.client.XClaim(ctx, &redis.XClaimArgs{
				Stream:   stream,
				Group:    c.group,
				Consumer: c.consumer,
				MinIdle:  c.opts.ClaimMinIdle,
				Messages: ids,
			}).Result()
			if err != nil {
				return fmt.Errorf("xclaim: %w", err)
			}

			for _, m := range messages {
				if retryCount, toDLQ := dlqIDs[m.ID]; toDLQ {
					if err := c.sendToDLQ(ctx, stream, &m, fmt.Sprintf("max retries exceeded: %d", retryCount)); err != nil {
						fmt.Printf("send to dlq error: %v\n", err)
						continue
					}
					if err := c.client.client.XAck(ctx, stream, c.group, m.ID).Err(); err != nil {
						fmt.Printf("ack dlq message error: %v\n", err)
					}
					continue
				}

				if err := c.processMessage(ctx, stream, m); err != nil {
					fmt.Printf("process pending message error: %v\n", err)
				}
			}
		}
	}
	return nil
}

// consume 消费新消息
func (c *Consumer) consume(ctx context.Context) error {
	// 构建 streams 参数
	args := make([]string, 0, len(c.streams)*2)
	for _, s := range c.streams {
		args = append(args, s)
	}
	for range c.streams {
		args = append(args, ">")
	}

	pendingTicker := time.NewTicker(c.opts.PendingCheckInterval)
	defer pendingTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pendingTicker.C:
			if err := c.processPending(ctx); err != nil && ctx.Err() == nil {
				fmt.Printf("process pending error: %v\n", err)
			}
		default:
		}

		// 读取消息
		results, err := c.client.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: c.consumer,
			Streams:  args,
			Count:    int64(c.opts.BatchSize),
			Block:    c.opts.BlockTime,
		}).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			return fmt.Errorf("xreadgroup: %w", err)
		}

		// 处理消息
		for _, result := range results {
			for _, m := range result.Messages {
				if err := c.processMessage(ctx, result.Stream, m); err != nil {
					fmt.Printf("process message error: %v\n", err)
				}
			}
		}
	}
}

// processMessage 处理单条消息
func (c *Consumer) processMessage(ctx context.Context, stream string, m redis.XMessage) error {
	data, ok := m.Values["data"].(string)
	if !ok {
		// 无效消息，直接 ACK
		return c.client.client.XAck(ctx, stream, c.group, m.ID).Err()
	}

	msg := &Message{
		ID:     m.ID,
		Stream: stream,
		Data:   []byte(data),
	}

	// 调用处理函数
	if err := c.handler(ctx, msg); err != nil {
		// 超过最大重试，写入死信流并 ACK
		if c.opts.MaxRetries > 0 {
			pending, pErr := c.client.client.XPendingExt(ctx, &redis.XPendingExtArgs{
				Stream: stream,
				Group:  c.group,
				Start:  m.ID,
				End:    m.ID,
				Count:  1,
			}).Result()
			if pErr == nil && len(pending) == 1 && pending[0].RetryCount > int64(c.opts.MaxRetries) {
				if dlqErr := c.sendToDLQ(ctx, stream, &m, err.Error()); dlqErr == nil {
					return c.client.client.XAck(ctx, stream, c.group, m.ID).Err()
				}
			}
		}
		return err
	}

	// ACK
	return c.client.client.XAck(ctx, stream, c.group, m.ID).Err()
}

func (c *Consumer) sendToDLQ(ctx context.Context, stream string, m *redis.XMessage, reason string) error {
	dlqStream := stream + ":dlq"
	values := map[string]interface{}{
		"stream":   stream,
		"msgId":    m.ID,
		"reason":   reason,
		"data":     m.Values["data"],
		"tsMs":     time.Now().UnixMilli(),
		"group":    c.group,
		"consumer": c.consumer,
	}
	_, err := c.client.client.XAdd(ctx, &redis.XAddArgs{
		Stream: dlqStream,
		Values: values,
	}).Result()
	if err != nil {
		return fmt.Errorf("xadd dlq: %w", err)
	}
	return nil
}

// Ack 手动确认消息
func (c *Consumer) Ack(ctx context.Context, stream, id string) error {
	return c.client.client.XAck(ctx, stream, c.group, id).Err()
}

// StreamInfo Stream 信息
type StreamInfo struct {
	Length         int64
	FirstEntry     string
	LastEntry      string
	ConsumerGroups int64
}

// Info 获取 Stream 信息
func (c *StreamClient) Info(ctx context.Context, stream string) (*StreamInfo, error) {
	info, err := c.client.XInfoStream(ctx, stream).Result()
	if err != nil {
		return nil, err
	}

	return &StreamInfo{
		Length:         info.Length,
		FirstEntry:     info.FirstEntry.ID,
		LastEntry:      info.LastEntry.ID,
		ConsumerGroups: int64(info.Groups),
	}, nil
}

// Trim 裁剪 Stream
func (c *StreamClient) Trim(ctx context.Context, stream string, maxLen int64) error {
	return c.client.XTrimMaxLen(ctx, stream, maxLen).Err()
}
