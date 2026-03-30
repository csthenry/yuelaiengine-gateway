/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-30 19:59:59
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-30 20:06:46
 * @FilePath: /yuelaiengine-gateway/internal/core/limiter/limiter.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package limiter

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Limiter 限流器接口
type Limiter interface {
	Allow(ctx context.Context, identifier string) bool
	Name() string
}

// IdentifierFunc 用于从 HTTP 请求中提取唯一标识符
type IdentifierFunc func(r *http.Request) string

// No-Op 如果没有开启限流器，可以使用 NoOpLimiter，其 Allow 方法总是返回 true
type NoOpLimiter struct {}

// GetIdentifierFunc 根据策略名称返回对应的标识符提取函数
func GetIdentifierFunc(strategy string) (IdentifierFunc, error) {
	switch strings.ToLower(strategy) {
	case "ip":
		return func(r *http.Request) string {
			// 优先从 X-Forwarded-For 获取，适配代理场景
			ip := r.Header.Get("X-Forwarded-For")
			if ip == "" {
				// X-Real-IP 是另一个常见Header
				ip = r.Header.Get("X-Real-Ip")
			}
			if ip == "" {
				// 最后回退到 RemoteAddr
				// 注意：RemoteAddr 可能包含端口，需要处理
				ip = strings.Split(r.RemoteAddr, ":")[0]
			}
			return ip
		}, nil
	case "path":
		return func(r *http.Request) string {
			return r.URL.Path
		}, nil
	case "global":
		return func(r *http.Request) string {
			return "global"
		}, nil
	default:
		return nil, fmt.Errorf("不支持的限流策略: '%s'", strategy)
	}
}

// Allow 返回 true
func (l *NoOpLimiter) Allow(ctx context.Context, identifier string) bool {
	return true
}

// Name 返回限流器名称
func (l *NoOpLimiter) Name() string {
	return "NoOpLimiter"
}