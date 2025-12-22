// Package service Redis 客户端接口
package service

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// RedisClient Redis 客户端接口，用于依赖注入和测试 mock
type RedisClient interface {
	XGroupCreateMkStream(ctx context.Context, stream, group, start string) *redis.StatusCmd
	XReadGroup(ctx context.Context, args *redis.XReadGroupArgs) *redis.XStreamSliceCmd
	XAck(ctx context.Context, stream, group string, ids ...string) *redis.IntCmd
}

// RedisClientAdapter 适配器，将 *redis.Client 适配为 RedisClient 接口
type RedisClientAdapter struct {
	client *redis.Client
}

// NewRedisClientAdapter 创建 Redis 客户端适配器
func NewRedisClientAdapter(client *redis.Client) RedisClient {
	return &RedisClientAdapter{client: client}
}

func (a *RedisClientAdapter) XGroupCreateMkStream(ctx context.Context, stream, group, start string) *redis.StatusCmd {
	return a.client.XGroupCreateMkStream(ctx, stream, group, start)
}

func (a *RedisClientAdapter) XReadGroup(ctx context.Context, args *redis.XReadGroupArgs) *redis.XStreamSliceCmd {
	return a.client.XReadGroup(ctx, args)
}

func (a *RedisClientAdapter) XAck(ctx context.Context, stream, group string, ids ...string) *redis.IntCmd {
	return a.client.XAck(ctx, stream, group, ids...)
}
