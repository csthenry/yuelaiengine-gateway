package core

import (
	"context"
	"errors"
	"sync"
	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/pkg/logger"
)

// Gateway API网关核心引擎
// 负责请求路由、负载均衡、健康检查和插件管理
type Gateway struct {
	mu sync.RWMutex

	config *config.GatewayConfig
	router *Router
	// [TODO]
	// healthChecker *health.HealthChecker
	// proxy             *Proxy
	// lbFactory         *loadbalancer.LoadBalancerFactory
	// pluginManager     *plugin.Manager
	// rateLimitSvc      svc_ratelimit.Service
	// circuitBreakerSvc svc_circuitbreaker.Service
	// cbHandler         *handler_cb.CircuitBreakerHandler
	logger logger.Logger

	reloadCancel context.CancelFunc
	reloadPath string
}

// NewGateway 创建网关实例，初始化组件
func NewGateway(cfg *config.GatewayConfig, logger logger.Logger) (*Gateway, error) {
	if cfg == nil {
		return nil, errors.New("gateway config is nil")
	}

	gw := &Gateway{
		config: cfg,
		logger: logger,
	}
	// [TODO]
	gw.mu.Lock()
	// if err := gw.applyConfigLocked(cfg.Clone()); err != nil {
	// 	gw.mu.Unlock()
	// 	return nil, err
	// }
	gw.mu.Unlock()

	logger.Info(context.Background(), "Gateway Core 初始化完成")
	return gw, nil
}