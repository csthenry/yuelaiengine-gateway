/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-30 16:40:33
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-30 20:34:25
 * @FilePath: /yuelaiengine-gateway/internal/core/service/circuitbreaker/service.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"yuelaiengine/gateway/pkg/logger"
)

// 全局错误定义
var (
	ErrOpenState       = errors.New("circuit breaker is open")              // 熔断器已经打开
	ErrTooManyRequests = errors.New("too mant requests")                    // 请求数超限
	ErrServiceNotFound = errors.New("service not found in circuit breaker") // 服务不存在
)

// State 熔断器状态枚举
type State int

const (
	StateClosed   State = iota // 关闭，运行请求，记录失败数
	StateHalfOpen              // 半开，允许少量请求，成功则关闭，失败则打开
	StateOpen                  // 全开， 拒接请求，等待超时后进入半开
)

// GetState 获取熔断器状态
func (s State) GetState() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateHalfOpen:
		return "half-open"
	case StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// CircuitState 熔断器状态的对外展示结构
type CircuitState struct {
	ServiceName      string    `json:"service_name"`             // 服务名
	State            string    `json:"state"`                    // 状态
	FailureCount     int       `json:"failure_count"`            // 失败次数
	SuccessCount     int       `json:"success_count"`            // 成功次数
	LastOpenTime     time.Time `json:"last_open_time,omitempty"` // 最后一次打开时间
	FailureThreshold int       `json:"failure_threshold"`        // 失败阈值（关闭时达到则打开）
	SuccessThreshold int       `json:"success_threshold"`        // 成功阈值（半开时达到则关闭）
	ResetTimeout     string    `json:"reset_timeout"`            // 重置超时时间（全开到半开的时间）
}

// Service 熔断器服务接口
type Service interface {
	CheckCircuit(ctx context.Context, serviceName string) (bool, error) // 检查是否允许请求
	RecordResult(ctx context.Context, serviceName string, success bool) // 记录后端服务请求结果
	GetAllState(ctx context.Context) map[string]CircuitState            // 获取所有熔断器状态
	OpenTransitionCount(ctx context.Context) uint64                     // 熔断器打开次数
	Reset(ctx context.Context, serviceName string) error                // 重置对应服务的熔断器
	Close(ctx context.Context) error                                    // 关闭熔断器服务
}

// CircuitBreaker 单个服务的熔断器实例
type CircuitBreaker struct {
	mu           sync.Mutex // 保护当前熔断器实例的并发安全
	state        State      // 当前状态
	failureCount int        // 失败次数
	successCount int        // 成功次数（主要用于半开状态）
	lastOpenTime time.Time  // 最后一次进入打开状态的时间
}

// service 熔断器服务实现
type service struct {
	mu               sync.RWMutex               // 保护多服务熔断器映射的并发安全
	circuitBreakers  map[string]*CircuitBreaker // 服务名 -> 熔断器实例的映射
	FailureThreshold int                        // 全局失败阈值（默认5次）
	SuccessThreshold int                        // 全局成功阈值（默认2次）
	ResetTimeout     time.Duration              // 全局重置超时时间（默认1分钟）
	openTransitions  atomic.Uint64
	logger           logger.Logger // 日志记录器
}

// 验证 Service 接口实现
var _ Service = (*service)(nil)

// NewService 创建熔断器服务实例
func NewService(failureThreshold int, successThreshold int, resetTimeout time.Duration, log logger.Logger) Service {
	// 默认值
	if failureThreshold <= 0 {
		failureThreshold = 5
	}
	if successThreshold <= 0 {
		successThreshold = 2
	}
	if resetTimeout <= 0 {
		resetTimeout = time.Minute
	}

	svc := &service{
		circuitBreakers:  make(map[string]*CircuitBreaker),
		FailureThreshold: failureThreshold,
		SuccessThreshold: successThreshold,
		ResetTimeout:     resetTimeout,
		logger:           log,
	}

	log.Info(context.Background(), "Circuit breaker service initialized",
		"failure_threshold", failureThreshold,
		"success_threshold", successThreshold,
		"reset_timeout", resetTimeout.String(),
		"service", "circuitbreaker")

	return svc
}

// GetAllState 返回所有熔断器状态
func (s *service) GetAllState(ctx context.Context) map[string]CircuitState {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[string]CircuitState, len(s.circuitBreakers))
	for serviceName, cb := range s.circuitBreakers {
		// 状态查询时也推进 open -> half-open，便于监控可观测到半开态。
		if cb.state == StateOpen && time.Since(cb.lastOpenTime) >= s.ResetTimeout {
			prevState := cb.state.GetState()
			cb.state = StateHalfOpen
			cb.failureCount = 0
			cb.successCount = 0
			s.logger.Info(ctx, "Circuit breaker state transition",
				"service_name", serviceName,
				"old_state", prevState,
				"new_state", cb.state.GetState(),
				"service", "circuitbreaker",
				"action", "state_transition_observe")
		}

		result[serviceName] = CircuitState{
			ServiceName:      serviceName,
			State:            cb.state.GetState(),
			FailureCount:     cb.failureCount,
			SuccessCount:     cb.successCount,
			LastOpenTime:     cb.lastOpenTime,
			FailureThreshold: s.FailureThreshold,
			SuccessThreshold: s.SuccessThreshold,
			ResetTimeout:     s.ResetTimeout.String(),
		}
	}

	s.logger.Debug(ctx, "Retrieved all circuit breaker states",
		"total_services", len(result),
		"service", "circuitbreaker",
		"action", "get_all_states")

	return result
}

// OpenTransitionCount 返回熔断器从非 open 切换到 open 的累计次数。
func (s *service) OpenTransitionCount(ctx context.Context) uint64 {
	return s.openTransitions.Load()
}

// Reset 重置对于服务的熔断器
func (s *service) Reset(ctx context.Context, serviceName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cb, exists := s.circuitBreakers[serviceName]
	if !exists {
		cb = &CircuitBreaker{
			state: StateClosed,
		}
		s.circuitBreakers[serviceName] = cb
		s.logger.Info(ctx, "Initialized circuit breaker for service during reset",
			"service_name", serviceName,
			"initial_state", "closed",
			"service", "circuitbreaker",
			"action", "reset_initialize")
	}

	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0

	s.logger.Info(ctx, "Circuit breaker reset successfully",
		"service_name", serviceName,
		"service", "circuitbreaker",
		"action", "reset_success")

	return nil

}

// CheckCircuit 检查指定服务的熔断器状态，返回是否允许请求
func (s *service) CheckCircuit(ctx context.Context, serviceName string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cb, exists := s.circuitBreakers[serviceName]
	if !exists {
		cb = &CircuitBreaker{
			state: StateClosed,
		}
		s.circuitBreakers[serviceName] = cb
		s.logger.Info(ctx, "Initialized circuit breaker for service",
			"service_name", serviceName,
			"initial_state", "closed",
			"service", "circuitbreaker",
			"action", "initialize")
	}

	// 检查熔断器状态
	switch cb.state {
	case StateOpen:
		// 大于重置时间则进入半开状态
		if time.Since(cb.lastOpenTime) >= s.ResetTimeout {
			prevState := cb.state
			cb.state = StateHalfOpen
			cb.failureCount = 0
			cb.successCount = 0
			s.logger.Info(ctx, "Circuit breaker state transition",
				"service_name", serviceName,
				"old_state", prevState,
				"new_state", cb.state.GetState(),
				"service", "circuitbreaker",
				"action", "state_transition")
			return true, nil // 半开状态允许试探请求
		} else {
			// 未到重置时间，处于全开状态
			s.logger.Debug(ctx, "Circuit breaker is open, request rejected",
				"service_name", serviceName,
				"time_since_open", time.Since(cb.lastOpenTime).String(),
				"reset_timeout", s.ResetTimeout.String(),
				"service", "circuitbreaker",
				"action", "request_rejected")
			return false, ErrOpenState
		}
	case StateHalfOpen:
		// 半开状态：允许试探请求
		s.logger.Debug(ctx, "Circuit breaker is half-open, allowing probe request",
			"service_name", serviceName,
			"service", "circuitbreaker",
			"action", "request_allowed")
		return true, nil
	case StateClosed:
		return true, nil
	default:
		// 未知状态：默认允许请求
		s.logger.Warn(ctx, "Circuit breaker state unknown, allowing request by default",
			"service_name", serviceName,
			"state", "unknown",
			"service", "circuitbreaker",
			"action", "request_allowed_fallback")
		return true, nil
	}
}

// RecordResult 记录后台服务的响应结果，更新熔断器状态
func (s *service) RecordResult(ctx context.Context, serviceName string, success bool) {
	s.mu.RLock()
	cb, exists := s.circuitBreakers[serviceName]
	s.mu.RUnlock()

	if !exists {
		s.logger.Warn(ctx, "Service circuit breaker not initialized, ignoring result recording",
			"service_name", serviceName,
			"success", success,
			"service", "circuitbreaker",
			"action", "record_ignored")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// 更新熔断器状态
	if success {
		cb.successCount++
		s.logger.Debug(ctx, "Service request succeeded",
			"service_name", serviceName,
			"success_count", cb.successCount,
			"current_state", cb.state.GetState(),
			"service", "circuitbreaker",
			"action", "record_success")
		// 半开状态如果达到了成功阈值，需要转换为关闭状态
		if cb.state == StateHalfOpen && cb.successCount >= s.SuccessThreshold {
			prevState := cb.state.GetState()
			cb.state = StateClosed
			cb.failureCount = 0
			cb.successCount = 0
			s.logger.Info(ctx, "Circuit breaker state transition",
				"service_name", serviceName,
				"old_state", prevState,
				"new_state", cb.state.GetState(),
				"success_threshold", s.SuccessThreshold,
				"service", "circuitbreaker",
				"action", "state_transition")
		}
	} else {
		// 请求失败
		cb.failureCount++
		s.logger.Debug(ctx, "Service request failed",
			"service_name", serviceName,
			"failure_count", cb.failureCount,
			"current_state", cb.state.GetState(),
			"service", "circuitbreaker",
			"action", "record_failure")

		// 关闭状态下，如果达到了失败阈值，需要转换为熔断
		if cb.state == StateClosed && cb.failureCount >= s.FailureThreshold {
			prevState := cb.state.GetState()
			cb.state = StateOpen
			s.openTransitions.Add(1)
			// 在熔断之前，需要记录熔断时间
			cb.lastOpenTime = time.Now()
			s.logger.Warn(ctx, "Circuit breaker state transition",
				"service_name", serviceName,
				"old_state", prevState,
				"new_state", cb.state.GetState(),
				"failure_threshold", s.FailureThreshold,
				"service", "circuitbreaker",
				"action", "state_transition")
		}
		// 半开状态下，失败就熔断
		if cb.state == StateHalfOpen {
			prevState := cb.state.GetState()
			cb.state = StateOpen
			s.openTransitions.Add(1)
			cb.lastOpenTime = time.Now()
			s.logger.Warn(ctx, "Circuit breaker state transition",
				"service_name", serviceName,
				"old_state", prevState,
				"new_state", cb.state.GetState(),
				"service", "circuitbreaker",
				"action", "state_transition")
		}
	}
}

// Close 关闭熔断器服务
func (s *service) Close(ctx context.Context) error {
	s.logger.Info(ctx, "Starting graceful shutdown of circuit breaker service",
		"total_services", len(s.circuitBreakers),
		"service", "circuitbreaker",
		"action", "shutdown_start")

	// 若添加了后台任务，可在此处通过 context 取消任务

	s.logger.Info(ctx, "Circuit breaker service shutdown completed",
		"service", "circuitbreaker",
		"action", "shutdown_complete")
	return nil
}
