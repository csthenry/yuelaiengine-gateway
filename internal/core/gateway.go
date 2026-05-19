/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-22 15:15:45
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-31 21:00:28
 * @FilePath: /yuelaiengine-gateway/internal/core/gateway.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package core

import (
	"context"
	"errors"
	"hash/fnv"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/core/health"
	"yuelaiengine/gateway/internal/core/loadbalancer"
	"yuelaiengine/gateway/internal/core/proxy"
	"yuelaiengine/gateway/internal/core/routingkey"
	handler_cb "yuelaiengine/gateway/internal/handler/circuitbreaker"
	"yuelaiengine/gateway/internal/plugin"
	"yuelaiengine/gateway/internal/plugin/httperr"
	svc_circuitbreaker "yuelaiengine/gateway/internal/service/circuitbreaker"
	svc_ratelimit "yuelaiengine/gateway/internal/service/ratelimit"

	"yuelaiengine/gateway/pkg/logger"
)

// Gateway API网关核心引擎
// 负责请求路由、负载均衡、健康检查和插件管理
type Gateway struct {
	mu sync.RWMutex

	config            *config.GatewayConfig
	router            *Router
	healthChecker     *health.HealthChecker
	lbFactory         *loadbalancer.LoadBalancerFactory
	pluginManager     *plugin.Manager
	proxy             *proxy.Proxy
	rateLimitSvc      svc_ratelimit.Service      // 限流服务
	circuitBreakerSvc svc_circuitbreaker.Service // 熔断器服务
	cbHandler         *handler_cb.CircuitBreakerHandler
	logger            logger.Logger

	reloadCancel  context.CancelFunc
	monitorCancel context.CancelFunc
	reloadPath    string
	monitorPath   string

	metrics *metricsCollector
	webUI   *webAssetServer

	versionCounter atomic.Uint64
	configHistory  []configSnapshot
	maxHistory     int
	runtimeSnap    atomic.Value // gatewayRuntimeSnapshot
}

type configSnapshot struct {
	Version   string
	Source    string
	CreatedAt time.Time
	Config    *config.GatewayConfig
}

type gatewayRuntimeSnapshot struct {
	config *config.GatewayConfig
	router *Router
}

// NewGateway 创建网关实例，初始化组件
func NewGateway(cfg *config.GatewayConfig, logger logger.Logger) (*Gateway, error) {
	if cfg == nil {
		return nil, errors.New("gateway config is nil")
	}

	// 初始化负载均衡器和服务健康检查
	lbFactory := loadbalancer.NewLoadBalancerFactory()
	healthChecker := health.NewHealthChecker(cfg.HealthCheck.Timeout, cfg.HealthCheck.Interval, logger)
	healthChecker.SetStatusChangeHook(func(serviceName, instanceURL string, isHealthy bool) {
		if err := lbFactory.UpdateInstanceAlive(serviceName, instanceURL, isHealthy); err != nil {
			logger.Error(context.Background(), "同步实例健康状态到负载均衡器失败",
				"service", serviceName,
				"instance", instanceURL,
				"healthy", isHealthy,
				"error", err.Error())
		}
	})

	// Gateway 部分服务在 runtime 中通过 plugin 注册
	proxy := proxy.NewProxy(lbFactory, healthChecker, nil, logger)
	pluginManager := plugin.NewManager(logger)

	gw := &Gateway{
		lbFactory:     lbFactory,
		healthChecker: healthChecker,
		pluginManager: pluginManager,
		proxy:         proxy,
		config:        cfg,
		logger:        logger,
		metrics:       newMetricsCollector(),
		webUI:         newWebAssetServer("./web/dist"),
		maxHistory:    20,
	}
	gw.runtimeSnap.Store(gatewayRuntimeSnapshot{})

	gw.mu.Lock()
	if err := gw.applyConfigLocked(cfg.Clone(), "bootstrap"); err != nil {
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
	start := time.Now()
	wrapper := &gatewayResponseWriter{ResponseWriter: w}
	defer func() {
		g.metrics.Observe(wrapper.StatusCode(), time.Since(start))
	}()

	ctx := r.Context()

	cfg, router := g.snapshot()

	// 查找匹配的路由
	route := router.FindRoute(r)
	if route == nil {
		// 路径存在但方法不允许，返回 405
		if matchedByPath := router.FindRouteByPathOnly(r.URL.Path); matchedByPath != nil {
			g.logger.Debug(ctx, "请求路径匹配但方法不允许",
				"method", r.Method,
				"path", r.URL.Path,
				"allowed_methods", matchedByPath.Methods)
			httperr.Write(wrapper, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "方法不允许")
			return
		}

		g.logger.Debug(ctx, "请求未匹配到任何路由", "method", r.Method, "path", r.URL.Path)
		httperr.Write(wrapper, http.StatusNotFound, "ROUTE_NOT_FOUND", "服务未找到")
		return
	}

	// 健康检查路由特殊处理
	if route.ServiceName == "all-services" {
		g.HealthCheckHandler(wrapper, r)
		return
	}

	// 选择服务
	targetServiceName := g.selectTargetService(cfg, route, r)
	service, exist := cfg.Services[targetServiceName]
	if !exist {
		g.logger.Debug(ctx, "请求匹配到路由但服务未在配置中定义", "method", r.Method, "path", r.URL.Path, "route", route.PathPrefix, "service", targetServiceName)
		httperr.Write(wrapper, http.StatusInternalServerError, "SERVICE_CONFIG_ERROR", "服务配置错误")
		return
	}
	g.logger.Debug(ctx, "请求匹配到路由", "method", r.Method, "path", r.URL.Path, "service", service.Name)

	// 插件链（支持 requires_auth 自动注入）
	pluginSpecs := clonePluginSpecs(route.Plugins)
	if route.RequiresAuth && !hasPlugin(pluginSpecs, "auth") {
		pluginSpecs = append(pluginSpecs, config.PluginSpec{"name": "auth"})
	}

	// 执行插件链
	continueChain, err := g.pluginManager.ExecuteChain(wrapper, r, pluginSpecs)
	if err != nil {
		g.logger.Error(ctx, "插件链执行因内部错误中断", "error", err)
		return
	}
	if !continueChain {
		g.logger.Debug(ctx, "插件链中断请求，处理结束")
		return
	}

	// 反向代理转发请求
	g.proxy.ServeHTTP(wrapper, r, route, &service)
}

// selectTargetService：按 A/B -> 权重 -> 默认服务 逐级选择
func (g *Gateway) selectTargetService(cfg *config.GatewayConfig, route *config.RouteConfig, r *http.Request) string {
	if route == nil {
		return ""
	}
	// A/B 策略，根据 Header 将流量转发至对应后台服务
	if route.ABHeader != "" && len(route.ABVariants) > 0 {
		val := strings.TrimSpace(r.Header.Get(route.ABHeader))
		if val != "" {
			if svc, ok := route.ABVariants[val]; ok {
				if _, exists := cfg.Services[svc]; exists {
					return svc
				}
			}
			if svc, ok := route.ABVariants[strings.ToLower(val)]; ok {
				if _, exists := cfg.Services[svc]; exists {
					return svc
				}
			}
		}
	}

	// 权重策略
	if len(route.TrafficWeights) > 0 {
		key := routingkey.ValueByStrategy(route.HashOn, r)
		if key == "" {
			key = r.URL.Path
		}
		if svc := selectWeightedService(route.TrafficWeights, key); svc != "" {
			if _, exists := cfg.Services[svc]; exists {
				return svc
			}
		}
	}

	return route.ServiceName
}

// selectWeightedService 根据权重比例，选择后端服务
func selectWeightedService(weights map[string]int, key string) string {
	if len(weights) == 0 {
		return ""
	}

	// 加权随机选择算法，和负载均衡器中的实现相同
	total := 0
	services := make([]string, 0, len(weights))
	for svc, w := range weights {
		if w > 0 {
			services = append(services, svc)
			total += w
		}
	}
	if total == 0 || len(services) == 0 {
		return ""
	}

	sort.Strings(services)
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	target := int(h.Sum32() % uint32(total))

	cumulative := 0
	for _, svc := range services {
		cumulative += weights[svc]
		if target < cumulative {
			return svc
		}
	}

	return services[0]
}

func hasPlugin(specs []config.PluginSpec, pluginName string) bool {
	for _, spec := range specs {
		// strings.EqualFold 用于不区分大小写比较字符串
		if name, ok := spec["name"].(string); ok && strings.EqualFold(name, pluginName) {
			return true
		}
	}
	return false
}

func clonePluginSpecs(src []config.PluginSpec) []config.PluginSpec {
	if len(src) == 0 {
		return nil
	}
	dst := make([]config.PluginSpec, len(src))
	copy(dst, src)
	return dst
}

type gatewayResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *gatewayResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *gatewayResponseWriter) StatusCode() int {
	if w.statusCode == 0 {
		return http.StatusOK
	}
	return w.statusCode
}

func (g *Gateway) nextVersionID() string {
	n := g.versionCounter.Add(1)
	return "v" + strconv.FormatUint(n, 10)
}
