/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-22 15:15:27
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-22 16:49:17
 * @FilePath: /yuelaiengine-gateway/internal/core/router.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package core

import (
	"net/http"
	"strings"
	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/pkg/logger"
)

// Roter 负责解析 HTTP 请求并匹配路由配置
type Router struct {
	// routes 存醋路由配置指针切片
	routes []*config.RouteConfig
	logger logger.Logger
}

func NewRouter(routes []*config.RouteConfig, logger logger.Logger) *Router {
	return &Router{
		routes: routes,
		logger: logger,
	}
}

// FindRoute 根据请求URL链路匹配路由配置
func (ro *Router) FindRoute(r *http.Request) *config.RouteConfig {
	route := ro.findRouteByPath(r.URL.Path)

	if route == nil {
		return nil
	}
	if !methodAllowed(route.Methods, r.Method) {
		return nil
	}
	return route
}

// FindRouteByPathOnly 按照请求路径查找路由
func (ro *Router) FindRouteByPathOnly(path string) *config.RouteConfig {
	return ro.findRouteByPath(path)
}

func (ro *Router) findRouteByPath(path string) *config.RouteConfig {
	// 采用最长前缀匹配
	var bestPrefixRoute *config.RouteConfig
	bestPrefixLen := -1

	for _, route := range ro.routes {
		if route == nil {
			continue
		}
		// 精确路径匹配
		if route.Path != "" && path == route.Path {
			return route
		}
		// 最长前缀路径匹配
		if route.PathPrefix != "" && prefixMatch(path, route.PathPrefix) {
			if len(route.PathPrefix) > bestPrefixLen {
				bestPrefixLen = len(route.PathPrefix)
				bestPrefixRoute = route
			}
		}
	}

	return bestPrefixRoute
}

// methodAllowed 请求方法合法检查
func methodAllowed(allowedMethods []string, method string) bool {
	if len(allowedMethods) == 0 {
		return true
	}
	for _, m := range allowedMethods {
		// strings.EqualFold 用于不区分大小写比较字符串
		if strings.EqualFold(strings.TrimSpace(m), method) {
			return true
		}
	}
	return false
}

// prefixMatch 路由前缀匹配
func prefixMatch(path, prefix string) bool {
	if prefix == "" || !strings.HasPrefix(path, prefix) {
		return false
	}

	// prefix[len(prefix)-1] == '/' 是处理 path=/user, prefix=/user/ 的情况
	if len(path) == len(prefix) || prefix[len(prefix)-1] == '/' {
		return true
	}

	// 确保 prefix 的下一个字符是 '/'，防止字符截断
	// path="/user/info", prefix="/user"，防止匹配 path="/username" 等情况
	return path[len(prefix)] == '/'
}