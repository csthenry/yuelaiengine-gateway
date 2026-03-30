/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-30 20:17:27
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-30 21:48:28
 * @FilePath: /yuelaiengine-gateway/internal/core/limiter/token_bucket.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
 package limiter


import (
	"context"
	"sync"
	"time"
)

// bucket 定义标识符状态
type bucket struct {
	tokens int
	lastCheck time.Time
}

// MemoryTokenBucket 基于内存的令牌桶限流器
type MemoryTokenBucket struct {
	name string
	capacity int
	refillRate int
	buckets map[string]*bucket
	mu sync.Mutex
}

// NewMemoryTokenBucket 创建一个新的内存令牌桶
func NewMemoryTokenBucket(capacity, refillRate int, name string) *MemoryTokenBucket {
	b := &MemoryTokenBucket{
		name: name,
		capacity: capacity,
		refillRate: refillRate,
		buckets: make(map[string]*bucket),
	}
	return b
}

func (b *MemoryTokenBucket) Allow(ctx context.Context, identifier string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 查找或创建
	currentBucket, ok := b.buckets[identifier]
	if !ok {
		// 首次访问，创建一个满的桶
		currentBucket = &bucket{
			tokens: b.capacity,
			lastCheck: time.Now(),
		}
		b.buckets[identifier] = currentBucket
	}

	// 补充 token
	now := time.Now()
	elapsed := now.Sub(currentBucket.lastCheck)
	refillCount := int(elapsed.Seconds() * float64(b.refillRate))

	if refillCount > 0 {
		currentBucket.tokens += refillCount
		currentBucket.lastCheck = now
	}
	if currentBucket.tokens > b.capacity {
		currentBucket.tokens = b.capacity
	}

	// 检查并消耗 token
	if currentBucket.tokens > 0 {
		currentBucket.tokens--
		return true
	}
	return false
}

// Name 返回限流器的名称
func (b *MemoryTokenBucket) Name() string {
	return b.name
}
