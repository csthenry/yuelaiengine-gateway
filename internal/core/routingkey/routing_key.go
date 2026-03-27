/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-26 22:46:00
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-26 22:54:54
 * @FilePath: /yuelaiengine-gateway/internal/core/routingkey/routing_key.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package routingkey

import (
	"net"
	"net/http"
	"strings"
)

// ValueByStrategy 根据策略提取路由选择使用的 Key
// 可用于灰度发布，根据 Request Header 对流量做区分转发
// 支持: ip/path/header:<name>/query:<name>/cookie:<name>
func ValueByStrategy(strategy string, r *http.Request) string {
	s := strings.TrimSpace(strings.ToLower(strategy))
	switch {
		// 常用于开发环境
	case s == "" , s == "ip":
		if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
		if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
			return xrip
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return r.RemoteAddr
		}
		return host
	case s == "path":
		return r.URL.Path
	case strings.HasPrefix(s, "header:"):
		key := strings.TrimSpace(strings.TrimPrefix(s, "header:"))
		return strings.TrimSpace(r.Header.Get(key))
	case strings.HasPrefix(s, "query:"):
		key := strings.TrimSpace(strings.TrimPrefix(s, "query:"))
		return strings.TrimSpace(r.URL.Query().Get(key))
	case strings.HasPrefix(s, "cookie:"):
		key := strings.TrimSpace(strings.TrimPrefix(s, "cookie:"))
		c, err := r.Cookie(key)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(c.Value)
	default:
		return ""
	}
}