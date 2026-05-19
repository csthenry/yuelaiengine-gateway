/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-26 22:55:29
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-04-03 11:15:49
 * @FilePath: /yuelaiengine-gateway/internal/core/proxy/proxy.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/core/loadbalancer"
	"yuelaiengine/gateway/internal/core/routingkey"
)

// ServerHTTP 反向代理服务器
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request, route *config.RouteConfig, service *config.ServiceConfig) {
	ctx := r.Context()

	if service == nil {
		p.logger.Error(ctx, "[Proxy] 服务配置错误", "route", route.PathPrefix)
		http.Error(w, "网关内部错误", http.StatusInternalServerError)
		return
	}

	// 获取负载均衡器
	lb := p.lbFactory.GetOrCreateLoadBalancer(service.Name, service.LoadBalancer)

	// 获取健康实例
	instance, err := p.getHealthyInstance(ctx, lb, service.Name, route, r)
	if err != nil {
		p.logger.Error(ctx, "[Proxy] 服务无可用实例", "service", service.Name, "error", err)
		// 无可用实例同样视作一次服务失败，用于触发熔断策略
		ref := p.circuitSvc.Load().(circuitSvcRef)
		cbSvc := ref.svc
		if cbSvc != nil {
			cbSvc.RecordResult(ctx, service.Name, false)
		}
		http.Error(w, fmt.Sprintf("服务 '%s' 当前不可用", service.Name), http.StatusBadGateway)
		return
	}
	p.logger.Debug(ctx, "[Proxy] 选择健康实例", "service", service.Name, "instance", instance.URL)

	// 如果负载均衡器采用 LC 算法，则需要进行连接释放
	if releaser, ok := lb.(connectionReleaser); ok {
		defer releaser.ReleaseConnection(service.Name, instance.URL)
	}

	// 创建反向代理
	targetURL, err := url.Parse(instance.URL)
	if err != nil {
		p.logger.Error(ctx, "[Proxy] 解析实例URL失败", "instance_url", instance.URL, "error", err)
		http.Error(w, "网关内部错误", http.StatusInternalServerError)
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	if strings.EqualFold(route.UpstreamProtocol, "grpc") {
		// 使用 H2C Transport
		proxy.Transport = grpcH2CTransport()
	} else if p.httpTransport != nil {
		// 对 HTTP 上游使用共享连接池，降低高并发下的建连抖动。
		proxy.Transport = p.httpTransport
	}
	// 创建 Transcoder，提供 HTTP JSON <--> gRPC 支持
	convertMode, routeTranscoder, err := p.prepareTranscoder(route)
	if err != nil {
		p.logger.Error(ctx, "[Proxy] 协议转换配置错误", "route", route.PathPrefix, "error", err)
		http.Error(w, "协议转换配置错误", http.StatusInternalServerError)
		return
	}

	// 请求体转换
	if routeTranscoder != nil {
		if err := applyRequestTranscoding(r, convertMode, routeTranscoder); err != nil {
			p.logger.Warn(ctx, "[Proxy] 请求体转码失败", "route", route.PathPrefix, "mode", convertMode, "error", err)
			http.Error(w, "请求体格式非法或不匹配 proto 描述", http.StatusBadRequest)
			return
		}
	}

	// 修改请求
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		// 执行默认的 host, scheme 等重写
		originalDirector(req)
		req.Host = targetURL.Host

		if strings.EqualFold(route.UpstreamProtocol, "grpc") {
			// TE: trailers 才代表是一个标准 gRPC 请求
			req.Header.Set("TE", "trailers")
			if route.GRPCMethod != "" {
				// 所有的 gRPC 请求必须是 POST
				req.Method = http.MethodPost
				// 外部 URL 替换成 gRPC 能识别的全限定方法名
				req.URL.Path = route.GRPCMethod
			}
		} else {
			originalPath := req.URL.Path
			if len(route.PathPrefix) > 0 && len(originalPath) >= len(route.PathPrefix) {
				// 移除路径前缀，保留剩余部分
				newPath := originalPath[len(route.PathPrefix):]
				if newPath == "" {
					newPath = "/"
				}
				req.URL.Path = newPath
				p.logger.Debug(req.Context(), "[Proxy] 路径重写", "original_path", originalPath, "new_path", newPath)
			}
		}

		switch convertMode {
		case "http_json_to_grpc":
			req.Method = http.MethodPost
			req.Header.Set("Content-Type", "application/grpc")
		case "grpc_to_http_json":
			req.Header.Set("Accept", "application/json")
		}
		req.Header.Set("X-Gateway-Proxy", "true")
	}

	// 处理代理错误
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
		var transcodeErr *proxyHTTPError
		if errors.As(proxyErr, &transcodeErr) {
			p.logger.Error(req.Context(), "[Proxy] 响应转码失败", "status", transcodeErr.StatusCode, "error", transcodeErr.Err)
			http.Error(rw, transcodeErr.Message, transcodeErr.StatusCode)
			return
		}
		p.logger.Error(req.Context(), "[Proxy] 代理转发失败", "error", proxyErr)
		http.Error(rw, "网关转发失败", http.StatusBadGateway)
	}

	// 重写响应体
	if routeTranscoder != nil {
		proxy.ModifyResponse = func(resp *http.Response) error {
			switch convertMode {
			case "http_json_to_grpc":
				return applyGRPCToJSONResponseTranscoding(resp, routeTranscoder)
			case "grpc_to_http_json":
				return applyJSONToGRPCResponseTranscoding(resp, routeTranscoder)
			default:
				return nil
			}
		}
	}

	// 使用 responseWriterWrapper 捕获响应状态码
	wrapper := &responseWriterWrapper{
		ResponseWriter: w,
		statusCode:     0,
	}

	// 执行代理
	proxy.ServeHTTP(wrapper, r)

	// 根据响应状态码更新熔断器状态
	statusCode := wrapper.GetStatusCode()
	success := statusCode >= 200 && statusCode < 400

	ref := p.circuitSvc.Load().(circuitSvcRef)
	cbSvc := ref.svc
	if cbSvc != nil {
		p.logger.Debug(ctx, "[Proxy] 服务请求完成", "service", service.Name, "status_code", statusCode, "success", success)
		cbSvc.RecordResult(ctx, service.Name, success)
	}
}

// getHealthyInstance 获取下一个健康实例
func (p *Proxy) getHealthyInstance(ctx context.Context, lb loadbalancer.LoadBalancer, serviceName string, route *config.RouteConfig, r *http.Request) (*loadbalancer.ServiceInstance, error) {
	allInstances := lb.GetAllInstances(serviceName)
	if len(allInstances) == 0 {
		return nil, errors.New("no instances available")
	}

	// 一致性哈希
	if hashLb, ok := lb.(hashSelector); ok {
		key := routingkey.ValueByStrategy(route.HashOn, r)
		if key == "" {
			key = r.URL.Path
		}
		if key != "" {
			instance, err := hashLb.GetInstanceByKey(serviceName, key)
			// 注意要检查实例健康状态
			if err == nil && p.healthChecker.IsInstanceHealthy(serviceName, instance.URL) {
				return instance, nil
			}
		}
	}

	// 使用负载均衡器获取一个健康实例
	// 这里其实不用 IsInstanceHealthy，因为在健康检查器中我做了回调函数
	// 会及时更新负载均衡器所维护的实例对象 Alive 字段
	for i := 0; i < len(allInstances); i++ {
		instance, err := lb.GetNextInstance(serviceName)
		if err != nil {
			return nil, err
		}
		if p.healthChecker.IsInstanceHealthy(serviceName, instance.URL) {
			return instance, nil
		}
		p.logger.Warn(ctx, "[Proxy] 实例异常", "instance", instance.URL, "service", serviceName)
	}
	return nil, errors.New("no healthy instances available")
}

// grpcH2CTransport 配置一个支持明文传输的 HTTP/2 客户端传输层
// HTTP/2 默认强制要求使用 TLS，但在受信任的内部微服务网络中，没有必要进行 TLS，就会使用 H2C(HTTP/2 Cleartext)
func grpcH2CTransport() *http2.Transport {
	return &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return (&net.Dialer{
				Timeout:   5 * time.Second, // 建立连接超时
				KeepAlive: 30 * time.Second,
			}).DialContext(ctx, network, addr)
		},
		IdleConnTimeout: 90 * time.Second,
	}
}

func newHTTPTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          2048,
		MaxIdleConnsPerHost:   512,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
