// Package queue 提供基于 Redis Stream 的通用消息队列（泛型 payload）。
//
// 从 iot-platform 基座 internal/queue/notify_queue.go 泛化抽取，机制保持一致：
//   - Stream 消费组：XADD（MaxLen 裁剪）+ XREADGROUP + XACK
//   - 失败重投：XPENDING + XCLAIM 认领超时消息，超过最大重试次数转入死信 Stream
//   - 延迟队列：ZSET + ZREM 抢占（多实例安全），到期后由调用方重新入队或直接处理
package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Options 队列配置
type Options struct {
	StreamKey     string        // Stream key，如 "notify:events"（必填）
	GroupName     string        // 消费组名，如 "notify-workers"（必填）
	MaxStreamLen  int64         // Stream 最大长度（近似裁剪），默认 10000
	ClaimMinIdle  time.Duration // 消息空闲多久后可被认领重投，默认 30s
	MaxClaimRetry int64         // 最大投递次数，超过转死信，默认 10
	DeadLetterKey string        // 死信 Stream key，空则超限消息直接 ACK 丢弃
	DelayedKey    string        // 延迟 ZSET key，空则禁用延迟能力
	NowFunc       func() time.Time // 当前时间函数，默认 time.Now（延迟队列判定到期用）
}

// Message 消费到的消息
type Message[T any] struct {
	ID      string // Stream 消息 ID，Ack 时使用
	Payload T
}

// Queue 泛型 Redis Stream 队列
type Queue[T any] struct {
	rdb  *redis.Client
	opts Options
}

// New 创建队列实例
func New[T any](rdb *redis.Client, opts Options) (*Queue[T], error) {
	if opts.StreamKey == "" || opts.GroupName == "" {
		return nil, fmt.Errorf("queue: StreamKey and GroupName are required")
	}
	if opts.MaxStreamLen <= 0 {
		opts.MaxStreamLen = 10000
	}
	if opts.ClaimMinIdle <= 0 {
		opts.ClaimMinIdle = 30 * time.Second
	}
	if opts.MaxClaimRetry <= 0 {
		opts.MaxClaimRetry = 10
	}
	if opts.NowFunc == nil {
		opts.NowFunc = time.Now
	}
	return &Queue[T]{rdb: rdb, opts: opts}, nil
}

// EnsureGroup 创建消费组（幂等，已存在时忽略 BUSYGROUP）
func (q *Queue[T]) EnsureGroup(ctx context.Context) error {
	err := q.rdb.XGroupCreateMkStream(ctx, q.opts.StreamKey, q.opts.GroupName, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("queue: create consumer group failed: %w", err)
	}
	return nil
}

// Enqueue 消息入队，返回 Stream 消息 ID
func (q *Queue[T]) Enqueue(ctx context.Context, payload *T) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("queue: marshal payload failed: %w", err)
	}
	id, err := q.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: q.opts.StreamKey,
		MaxLen: q.opts.MaxStreamLen,
		Approx: true,
		Values: map[string]interface{}{"data": string(data)},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("queue: XADD failed: %w", err)
	}
	return id, nil
}

// EnqueueDelayed 写入延迟 ZSET，到 dueAt 后由 PopDue 取出
func (q *Queue[T]) EnqueueDelayed(ctx context.Context, payload *T, dueAt time.Time) error {
	if q.opts.DelayedKey == "" {
		return fmt.Errorf("queue: DelayedKey not configured")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("queue: marshal delayed payload failed: %w", err)
	}
	if err := q.rdb.ZAdd(ctx, q.opts.DelayedKey, redis.Z{
		Score:  float64(dueAt.Unix()),
		Member: string(data),
	}).Err(); err != nil {
		return fmt.Errorf("queue: delayed ZADD failed: %w", err)
	}
	return nil
}

// PopDue 取出已到期的延迟消息，ZREM 抢占保证多实例下每条仅被一个实例消费
func (q *Queue[T]) PopDue(ctx context.Context, limit int64) ([]T, error) {
	if q.opts.DelayedKey == "" {
		return nil, nil
	}
	now := q.opts.NowFunc().Unix()
	members, err := q.rdb.ZRangeByScore(ctx, q.opts.DelayedKey, &redis.ZRangeBy{
		Min:   "-inf",
		Max:   fmt.Sprintf("%d", now),
		Count: limit,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("queue: delayed ZRANGEBYSCORE failed: %w", err)
	}

	var payloads []T
	for _, member := range members {
		// ZREM 返回 1 表示当前实例抢到了这条消息
		removed, err := q.rdb.ZRem(ctx, q.opts.DelayedKey, member).Result()
		if err != nil || removed == 0 {
			continue
		}
		var payload T
		if err := json.Unmarshal([]byte(member), &payload); err != nil {
			continue
		}
		payloads = append(payloads, payload)
	}
	return payloads, nil
}

// ReadBatch 消费组批量读取新消息
func (q *Queue[T]) ReadBatch(ctx context.Context, consumer string, count int64, blockTimeout time.Duration) ([]Message[T], error) {
	results, err := q.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    q.opts.GroupName,
		Consumer: consumer,
		Streams:  []string{q.opts.StreamKey, ">"},
		Count:    count,
		Block:    blockTimeout,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("queue: XREADGROUP failed: %w", err)
	}
	return q.parseMessages(results), nil
}

// Ack 确认消息
func (q *Queue[T]) Ack(ctx context.Context, ids ...string) error {
	return q.rdb.XAck(ctx, q.opts.StreamKey, q.opts.GroupName, ids...).Err()
}

// ClaimStaleMessages 认领超时未 ACK 的消息重投；投递次数超限的转入死信
func (q *Queue[T]) ClaimStaleMessages(ctx context.Context, consumer string, count int64) ([]Message[T], error) {
	pending, err := q.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: q.opts.StreamKey,
		Group:  q.opts.GroupName,
		Idle:   q.opts.ClaimMinIdle,
		Start:  "-",
		End:    "+",
		Count:  count,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("queue: XPENDING failed: %w", err)
	}
	if len(pending) == 0 {
		return nil, nil
	}

	var toDeadLetter []string
	var toClaim []string
	for _, p := range pending {
		if p.RetryCount >= q.opts.MaxClaimRetry {
			toDeadLetter = append(toDeadLetter, p.ID)
		} else {
			toClaim = append(toClaim, p.ID)
		}
	}

	if len(toDeadLetter) > 0 {
		q.moveToDeadLetter(ctx, toDeadLetter)
	}
	if len(toClaim) == 0 {
		return nil, nil
	}

	claimed, err := q.rdb.XClaim(ctx, &redis.XClaimArgs{
		Stream:   q.opts.StreamKey,
		Group:    q.opts.GroupName,
		Consumer: consumer,
		MinIdle:  q.opts.ClaimMinIdle,
		Messages: toClaim,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("queue: XCLAIM failed: %w", err)
	}
	return q.parseMessages([]redis.XStream{{Messages: claimed}}), nil
}

// moveToDeadLetter 将消息转入死信 Stream 并 ACK 原消息
func (q *Queue[T]) moveToDeadLetter(ctx context.Context, ids []string) {
	for _, id := range ids {
		msgs, err := q.rdb.XRangeN(ctx, q.opts.StreamKey, id, id, 1).Result()
		if err != nil || len(msgs) == 0 {
			_ = q.rdb.XAck(ctx, q.opts.StreamKey, q.opts.GroupName, id).Err()
			continue
		}
		if q.opts.DeadLetterKey != "" {
			fields := msgs[0].Values
			fields["original_id"] = id
			fields["dead_reason"] = "max_retry_exceeded"
			fields["dead_at"] = q.opts.NowFunc().Unix()

			_ = q.rdb.XAdd(ctx, &redis.XAddArgs{
				Stream: q.opts.DeadLetterKey,
				MaxLen: 5000,
				Approx: true,
				Values: fields,
			}).Err()
		}
		_ = q.rdb.XAck(ctx, q.opts.StreamKey, q.opts.GroupName, id).Err()
	}
}

// parseMessages 解析 Stream 消息中的 data 字段为 payload
func (q *Queue[T]) parseMessages(results []redis.XStream) []Message[T] {
	var messages []Message[T]
	for _, stream := range results {
		for _, msg := range stream.Messages {
			data, ok := msg.Values["data"].(string)
			if !ok {
				continue
			}
			var payload T
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				continue
			}
			messages = append(messages, Message[T]{ID: msg.ID, Payload: payload})
		}
	}
	return messages
}
