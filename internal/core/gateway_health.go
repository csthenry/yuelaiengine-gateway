/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-24 20:37:15
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-24 20:53:56
 * @FilePath: /yuelaiengine-gateway/internal/core/gateway_health.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package core

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"yuelaiengine/gateway/internal/config"
)

// HealthCheckHandler 健康检查
func (gw *Gateway) HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg, router := gw.snapshot()

	// 获取配置
	route := router.FindRouteByPathOnly(r.URL.Path)
	if route == nil {
		http.Error(w, "路由未找到", http.StatusNotFound)
		return
	}

	var resp interface{}
	if route.HealthCheckScope == "auto" {
		// 根据端口选择监测范围
		port := extractPort(r.Host)
		gatewayPort := strings.TrimPrefix(cfg.Server.Port, ":")

		// 没有端口，或者检查网关端口，都应该获取所有服务的健康状态
		// 对于一个网关来说，它自身的健康不仅仅是自身，更重要的是其代理的所有服务
		if port == "" || port == gatewayPort {
			resp = gw.healthChecker.GetAllStatuses()
		} else {
			resp = gw.buildServiceHealthResponse(cfg, route.ServiceName)
		}
	} else if route.HealthCheckScope == "all-services" {
		resp = gw.healthChecker.GetAllStatuses()
	} else {
		resp = gw.buildServiceHealthResponse(cfg, route.ServiceName)
	}

	if resp == nil {
		resp = gw.healthChecker.GetAllStatuses()
	}

	// http respond
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		gw.logger.Error(ctx, "写入健康检查失败", "error", err)
	}
}

// buildServiceHealthResponse 健康检查结果
func (g *Gateway) buildServiceHealthResponse(cfg *config.GatewayConfig, serviceName string) interface{} {
	if serviceName == "all-services" {
		return g.healthChecker.GetAllStatuses()
	}

	if _, exists := cfg.Services[serviceName]; !exists {
		return nil
	}

	return map[string]interface{}{
		serviceName: g.healthChecker.GetServiceStatus(serviceName),
	}
}

// extractPort 提取端口
func extractPort(host string) string {
	if host == "" {
		return ""
	}

	_, port, err := net.SplitHostPort(host)
	if err == nil {
		return port
	}

	// 兼容 Host 仅包含主机名、或无效格式场景。
	if idx := strings.LastIndex(host, ":"); idx >= 0 && idx < len(host)-1 {
		return host[idx+1:]
	}
	return ""
}