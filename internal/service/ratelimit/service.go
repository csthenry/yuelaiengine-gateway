/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-30 19:56:53
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-30 21:46:00
 * @FilePath: /yuelaiengine-gateway/internal/service/ratelimit/service.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package ratelimit

import (
	"context"
	"fmt"
	"sync"

	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/core/limiter"
	"yuelaiengine/gateway/pkg/logger"
)

// Service 限流器接口
type Service interface {
	CheckLimit(ctx context.Context, ruleName, identifier string) (bool, error)
	Close() error
}

// service 限流器具体实现
type service struct {
	mu sync.RWMutex
	limiters map[string]limiter.Limiter
	logger logger.Logger
}

// 验证 Service 接口实现
var _ Service = (*service)(nil)

// NewService 创建限流器实例
func NewService(cfg config.RateLimitingConfig, log logger.Logger) (Service, error) {
	ctx := context.Background()

	s := &service{
		limiters: make(map[string]limiter.Limiter),
		logger: log,
	}
	log.Info(ctx, "Initializing rate limit service",
		"total_rules", len(cfg.Rules),
		"service", "ratelimit",
		"action", "initialize")

	for _, rule := range cfg.Rules {
		// 拷贝一个 rule 对象，防止闭包
		currentRule := rule

		var lim limiter.Limiter
		var err error

		switch currentRule.Type{
		case "memory_token_bucket":
			// 使用新的构造函数，并传入 service 的 context。
			lim = limiter.NewMemoryTokenBucket(
				currentRule.TokenBucket.Capacity,
				currentRule.TokenBucket.RefillRate,
				currentRule.Name,
			)
		case "", "noop":
			lim = &limiter.NoOpLimiter{}
		default:
			err = fmt.Errorf("未知的限流器类型: %s for rule %s", currentRule.Type, currentRule.Name)
		}
		if err != nil {
			// 任何一个限流器创建失败，则立即取消上下文并返回错误
			log.Error(ctx, "Failed to initialize rate limiter",
				"rule_name", currentRule.Name,
				"limiter_type", currentRule.Type,
				"error", err.Error(),
				"service", "ratelimit",
				"action", "initialization_failed")
			return nil, err
		}
		s.limiters[currentRule.Name] = lim
		log.Info(ctx, "Successfully initialized rate limit rule",
			"rule_name", currentRule.Name,
			"limiter_type", lim.Name(),
			"service", "ratelimit",
			"action", "rule_initialized")
	}
	log.Info(ctx, "Rate limit service initialization completed",
		"active_limiters", len(s.limiters),
		"service", "ratelimit",
		"action", "initialization_completed")

	return s, nil
}

// CheckLimit 检查给定的标识符是否被特定规则允许
func (s *service) CheckLimit(ctx context.Context, ruleName, identifier string) (bool, error) {
	s.mu.RLock()
	lim, ok := s.limiters[ruleName]
	s.mu.RUnlock()

	if !ok {
		s.logger.Error(ctx, "Rate limit rule not found",
			"rule_name", ruleName,
			"identifier", identifier,
			"service", "ratelimit",
			"action", "rule_not_found")
		return false, fmt.Errorf("限流规则 '%s' 未定义", ruleName)
	}

	isAllowed := lim.Allow(ctx, identifier)

	if isAllowed {
		s.logger.Debug(ctx, "Rate limit check passed",
			"rule_name", ruleName,
			"identifier", identifier,
			"limiter_type", lim.Name(),
			"service", "ratelimit",
			"action", "check_passed")
	} else {
		s.logger.Debug(ctx, "Rate limit check failed - request blocked",
			"rule_name", ruleName,
			"identifier", identifier,
			"limiter_type", lim.Name(),
			"service", "ratelimit",
			"action", "check_failed")
	}

	return isAllowed, nil
}

// Close 关闭限流器
func (s *service) Close() error {
	ctx := context.Background()

	s.logger.Info(ctx, "Starting graceful shutdown of rate limit service",
		"active_limiters", len(s.limiters),
		"service", "ratelimit",
		"action", "shutdown_start")

	s.logger.Info(ctx, "Rate limit service shutdown completed",
		"service", "ratelimit",
		"action", "shutdown_completed")

	return nil
}
