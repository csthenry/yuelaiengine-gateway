/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-22 15:15:45
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-24 21:24:30
 * @FilePath: /yuelaiengine-gateway/internal/core/gateway.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package core

import (
	"net/http"
	"context"
	"errors"
	"sync"
	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/core/health"
	"yuelaiengine/gateway/pkg/logger"
)

// Gateway API网关核心引擎
// 负责请求路由、负载均衡、健康检查和插件管理
type Gateway struct {
	mu sync.RWMutex

	config *config.GatewayConfig
	router *Router
	healthChecker *health.HealthChecker
	// [TODO]
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

	// [TODO]
	healthChecker := health.NewHealthChecker(cfg.HealthCheck.Timeout, cfg.HealthCheck.Interval, logger)
	// healthChecker := health.NewHealthChecker(cfg.HealthCheck.Timeout, cfg.HealthCheck.Interval, log)
	// pluginManager := plugin.NewManager(log)
	// proxy := NewProxy(lbFactory, healthChecker, nil, log)

	gw := &Gateway{
		healthChecker: healthChecker,
		// pluginManager: pluginManager,
		// proxy:         proxy,
		config: cfg,
		logger: logger,
	}

	gw.mu.Lock()
	if err := gw.applyConfigLocked(cfg.Clone()); err != nil {
		gw.mu.Unlock()
		return nil, err
	}
	gw.mu.Unlock()

	// 启动健康检查
	go healthChecker.Start()

	logger.Info(context.Background(), "Gateway Core 初始化完成")
	return gw, nil
}

// ServeHTTP 网关请求处理入口，实现 http.Handler 接口
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// [TODO]
	// if strings.HasPrefix(r.URL.Path, "/admin/") {
	// 	g.handleAdminRequest(w, r)
	// 	return
	// }

	_, router := g.snapshot()

	// 查找匹配的路由
	route := router.FindRoute(r)
	if route == nil {
		// 路径存在但方法不允许，返回 405
		if matchedByPath := router.FindRouteByPathOnly(r.URL.Path); matchedByPath != nil {
			g.logger.Info(ctx, "请求路径匹配但方法不允许",
				"method", r.Method,
				"path", r.URL.Path,
				"allowed_methods", matchedByPath.Methods)
			http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
			return
		}

		g.logger.Info(ctx, "请求未匹配到任何路由", "method", r.Method, "path", r.URL.Path)
		http.Error(w, "服务未找到", http.StatusNotFound)
		return
	}

	// 健康检查路由特殊处理
	if route.ServiceName == "all-services" {
		g.HealthCheckHandler(w, r)
		return
	}

	// [TODO] Route

	// [TODO] Auth Plugin

	// [TODO] 执行插件链

	// [TODO] 反向代理转发请求
	// g.proxy.ServeHTTP(w, r, route, &service)
}
