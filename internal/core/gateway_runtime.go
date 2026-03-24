/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-24 20:41:01
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-24 21:57:24
 * @FilePath: /yuelaiengine-gateway/internal/core/gateway_runtime.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"yuelaiengine/gateway/internal/config"
)

// snapshot 返回网关当前快照
func (g *Gateway) snapshot() (*config.GatewayConfig, *Router) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.config, g.router
}

// Shutdown 优雅关闭网关
// 停止健康检查和所有服务
func (g *Gateway) Shutdown() {
	ctx := context.Background()
	g.logger.Info(ctx, "网关正在关闭...")
	g.StopAutoReload()

	// 停止健康检查
	g.healthChecker.Shutdown()

	g.mu.Lock()
	defer g.mu.Unlock()

	// 关闭限流服务
	// [tODO]

	// 关闭熔断器服务
	// [TODO]

	g.logger.Info(ctx, "网关已成功关闭。")
}

// StartAutoReload 开启配置热更新。
func (g *Gateway) StartAutoReload(configPath string, interval time.Duration) {
	if configPath == "" {
		return
	}
	if interval <= 0 {
		interval = 3 * time.Second
	}

	g.StopAutoReload()

	ctx, cancel := context.WithCancel(context.Background())
	g.mu.Lock()
	g.reloadCancel = cancel
	g.reloadPath = configPath
	g.mu.Unlock()

	go func() {
		var lastModTime time.Time
		if fi, err := os.Stat(configPath); err == nil {
			lastModTime = fi.ModTime()
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fi, err := os.Stat(configPath)
				if err != nil {
					g.logger.Warn(context.Background(), "热更新检查配置文件失败", "path", configPath, "error", err)
					continue
				}
				if fi.ModTime().After(lastModTime) {
					if err := g.ReloadFromFile(configPath); err != nil {
						g.logger.Error(context.Background(), "配置热更新失败", "path", configPath, "error", err)
					} else {
						lastModTime = fi.ModTime()
						g.logger.Info(context.Background(), "配置热更新成功", "path", configPath)
					}
				}
			}
		}
	}()
}

// StopAutoReload 停止热更新协程。
func (g *Gateway) StopAutoReload() {
	g.mu.Lock()
	cancel := g.reloadCancel
	g.reloadCancel = nil
	g.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ReloadFromFile 手动从配置文件重载。
func (g *Gateway) ReloadFromFile(path string) error {
	if path == "" {
		g.mu.RLock()
		path = g.reloadPath
		g.mu.RUnlock()
	}
	if path == "" {
		path = "./configs/config.yml"
	}

	newCfg, err := config.Load(path)
	if err != nil {
		return err
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.applyConfigLocked(newCfg.Clone()); err != nil {
		return err
	}
	g.reloadPath = path
	return nil
}

func (g *Gateway) applyConfigLocked(newCfg *config.GatewayConfig) error {
	if newCfg == nil {
		return errors.New("new config is nil")
	}
	if err := validateRouteTranscodingConfigs(newCfg.Routes); err != nil {
		return err
	}

	oldCfg := g.config

	// [TODO] 限流和熔断
	// oldRateLimitSvc := g.rateLimitSvc
	// oldCircuitSvc := g.circuitBreakerSvc

	// newRateLimitSvc, err := svc_ratelimit.NewService(newCfg.RateLimiting, g.logger)
	// if err != nil {
	// 	return fmt.Errorf("初始化限流服务失败: %w", err)
	// }

	// newCircuitSvc := svc_circuitbreaker.NewService(
	// 	newCfg.CircuitBreaker.FailureThreshold,
	// 	newCfg.CircuitBreaker.SuccessThreshold,
	// 	newCfg.CircuitBreaker.ResetTimeout,
	// 	g.logger,
	// )

	if err := g.syncServicesLocked(oldCfg, newCfg); err != nil {
		// _ = newRateLimitSvc.Close()
		return err
	}

	// [TODO] 刷新插件依赖
	// rateLimitPlugin := pl_ratelimit.NewPlugin(newRateLimitSvc, g.logger)
	// g.pluginManager.Register(rateLimitPlugin)

	// authPlugin, err := pl_auth.NewPlugin(g.lbFactory, g.healthChecker, "auth-service", newCfg.AuthService.ValidateURL, g.logger)
	// if err != nil {
	// 	_ = newRateLimitSvc.Close()
	// 	return fmt.Errorf("初始化认证插件失败: %w", err)
	// }
	// g.pluginManager.Register(authPlugin)
	// g.pluginManager.Register(pl_circuitbreaker.NewPlugin(newCircuitSvc, g.logger))
	// g.pluginManager.Register(pl_apikey.NewPlugin(g.logger))
	// g.pluginManager.Register(pl_rbac.NewPlugin(g.logger))

	// g.rateLimitSvc = newRateLimitSvc
	// g.circuitBreakerSvc = newCircuitSvc
	// g.proxy.UpdateCircuitBreakerService(newCircuitSvc)
	g.config = newCfg
	g.router = NewRouter(newCfg.Routes, g.logger)
	// g.cbHandler = handler_cb.NewCircuitBreakerHandler(newCfg, newCircuitSvc, g.logger)

	// if oldRateLimitSvc != nil {
	// 	_ = oldRateLimitSvc.Close()
	// }
	// if oldCircuitSvc != nil {
	// 	_ = oldCircuitSvc.Close(context.Background())
	// }

	return nil
}

func (g *Gateway) syncServicesLocked(oldCfg, newCfg *config.GatewayConfig) error {
	// [TODO] lbFactory
	for _, serviceCfg := range newCfg.Services {
		instanceURLs := make([]string, 0, len(serviceCfg.Instances))
		for _, inst := range serviceCfg.Instances {
			instanceURLs = append(instanceURLs, inst.URL)
		}

		g.healthChecker.RegisterService(serviceCfg.Name, instanceURLs, serviceCfg.HealthCheckPath)
		// g.lbFactory.ReplaceServiceInstances(serviceCfg.Name, serviceCfg.LoadBalancer, instances)
	}

	if oldCfg != nil {
		for oldService := range oldCfg.Services {
			if _, stillExists := newCfg.Services[oldService]; !stillExists {
				g.healthChecker.RemoveService(oldService)
				// g.lbFactory.RemoveService(oldService)
			}
		}
	}

	return nil
}

func validateRouteTranscodingConfigs(routes []*config.RouteConfig) error {
	for _, route := range routes {
		if route == nil {
			continue
		}

		mode := strings.ToLower(strings.TrimSpace(route.ProtocolConvert))
		if mode == "" || mode == "none" {
			continue
		}

		switch mode {
		case "http_json_to_grpc", "grpc_to_http_json":
		default:
			return fmt.Errorf("路由 %q 配置了不支持的 protocol_convert=%q", route.PathPrefix, route.ProtocolConvert)
		}

		if strings.TrimSpace(route.GRPCMethod) == "" {
			return fmt.Errorf("路由 %q 启用 %s 时必须配置 grpc_method", route.PathPrefix, mode)
		}
		if strings.TrimSpace(route.ProtoDescriptor) == "" {
			return fmt.Errorf("路由 %q 启用 %s 时必须配置 proto_descriptor_path", route.PathPrefix, mode)
		}
	}
	return nil
}
