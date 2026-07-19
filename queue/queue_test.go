package queue

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type testPayload struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func newTestQueue(t *testing.T) (*Queue[testPayload], *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	q, err := New[testPayload](rdb, Options{
		StreamKey:     "test:events",
		GroupName:     "test-workers",
		DeadLetterKey: "test:deadletter",
		DelayedKey:    "test:delayed",
		MaxClaimRetry: 2,
		ClaimMinIdle:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.EnsureGroup(context.Background()); err != nil {
		t.Fatal(err)
	}
	return q, mr
}

func TestEnqueueReadAck(t *testing.T) {
	q, _ := newTestQueue(t)
	ctx := context.Background()

	id, err := q.Enqueue(ctx, &testPayload{ID: 1, Name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("empty message id")
	}

	msgs, err := q.ReadBatch(ctx, "c1", 10, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Payload.ID != 1 || msgs[0].Payload.Name != "a" {
		t.Fatalf("msgs = %+v", msgs)
	}
	if err := q.Ack(ctx, msgs[0].ID); err != nil {
		t.Fatal(err)
	}

	// ACK 后不应再读到
	msgs, _ = q.ReadBatch(ctx, "c1", 10, time.Millisecond)
	if len(msgs) != 0 {
		t.Errorf("expected no messages after ack, got %d", len(msgs))
	}
}

func TestEnsureGroupIdempotent(t *testing.T) {
	q, _ := newTestQueue(t)
	// 重复创建组不应报错
	if err := q.EnsureGroup(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDelayedQueue(t *testing.T) {
	q, mr := newTestQueue(t)
	ctx := context.Background()

	// 未到期不弹出
	future := time.Now().Add(time.Hour)
	if err := q.EnqueueDelayed(ctx, &testPayload{ID: 2}, future); err != nil {
		t.Fatal(err)
	}
	got, err := q.PopDue(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected no due messages, got %d", len(got))
	}

	// 已到期弹出
	_ = mr // miniredis 不需要 FastForward，直接用过去时间入队
	if err := q.EnqueueDelayed(ctx, &testPayload{ID: 3}, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	got, err = q.PopDue(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != 3 {
		t.Fatalf("got = %+v", got)
	}

	// 弹出后再次 Pop 应为空（ZREM 抢占）
	got, _ = q.PopDue(ctx, 10)
	if len(got) != 0 {
		t.Errorf("expected empty after pop, got %d", len(got))
	}
}

func TestClaimAndDeadLetter(t *testing.T) {
	q, mr := newTestQueue(t)
	ctx := context.Background()

	if _, err := q.Enqueue(ctx, &testPayload{ID: 9}); err != nil {
		t.Fatal(err)
	}
	// c1 读取但不 ACK
	msgs, err := q.ReadBatch(ctx, "c1", 10, time.Millisecond)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("read failed: %v, %d", err, len(msgs))
	}

	// 等待超过 ClaimMinIdle 后由 c2 认领（第 2 次投递）
	mr.SetTime(time.Now().Add(time.Second))
	claimed, err := q.ClaimStaleMessages(ctx, "c2", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].Payload.ID != 9 {
		t.Fatalf("claimed = %+v", claimed)
	}

	// 再次超时认领：RetryCount 已达 MaxClaimRetry(2)，应转入死信
	mr.SetTime(time.Now().Add(2 * time.Second))
	claimed, err = q.ClaimStaleMessages(ctx, "c3", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 0 {
		t.Fatalf("expected dead letter, but claimed = %+v", claimed)
	}

	// 死信 Stream 应有 1 条
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	n, err := rdb.XLen(ctx, "test:deadletter").Result()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("deadletter len = %d", n)
	}
}
