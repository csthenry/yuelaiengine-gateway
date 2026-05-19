/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-26 22:22:57
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-31 20:00:56
 * @FilePath: /yuelaiengine-gateway/internal/core/proxy/types.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package proxy

import (
	"net/http"
	"sync"
	"sync/atomic"

	"yuelaiengine/gateway/internal/core/health"
	"yuelaiengine/gateway/internal/core/loadbalancer"
	"yuelaiengine/gateway/internal/core/transcoding"
	"yuelaiengine/gateway/internal/service/circuitbreaker"
	"yuelaiengine/gateway/pkg/logger"
)

// Proxy 转发请求到后台
type Proxy struct {
	lbFactory        *loadbalancer.LoadBalancerFactory
	healthChecker    *health.HealthChecker
	logger           logger.Logger
	circuitSvc       atomic.Value // circuitSvcRef
	descriptorLoader *transcoding.DescriptorResolver
	httpTransport    *http.Transport
	transcoderCache  sync.Map // key(string) -> *transcoding.RouteTranscoder
}

type circuitSvcRef struct {
	svc circuitbreaker.Service
}

type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

type connectionReleaser interface {
	ReleaseConnection(serviceName, instanceURL string) error
}

type hashSelector interface {
	GetInstanceByKey(serviceName, key string) (*loadbalancer.ServiceInstance, error)
}

func NewProxy(lbFactory *loadbalancer.LoadBalancerFactory, hc *health.HealthChecker, cbSvc circuitbreaker.Service, logger logger.Logger) *Proxy {
	p := &Proxy{
		lbFactory:        lbFactory,
		healthChecker:    hc,
		descriptorLoader: transcoding.NewDescriptorResolver(),
		httpTransport:    newHTTPTransport(),
		logger:           logger,
	}
	p.circuitSvc.Store(circuitSvcRef{svc: cbSvc})
	return p
}

// UpdateCircuitBreakerService 动态更新熔断器服务依赖，用于配置热更新
func (p *Proxy) UpdateCircuitBreakerService(svr circuitbreaker.Service) {
	p.circuitSvc.Store(circuitSvcRef{svc: svr})
}

func (w *responseWriterWrapper) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterWrapper) GetStatusCode() int {
	if w.statusCode == 0 {
		return http.StatusOK
	}
	return w.statusCode
}

type proxyHTTPError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *proxyHTTPError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Message
}
