/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-22 15:17:16
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-22 16:14:18
 * @FilePath: /yuelaiengine-gateway/internal/config/config.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
// package config 定义了网关YAML配置的结构体和加载逻辑。
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

// GatewayConfig 是整个网关配置的根结构
type GatewayConfig struct {
	Server         ServerConfig             `yaml:"server"`
	HealthCheck    HealthCheckConfig        `yaml:"health_check"`
	Services       map[string]ServiceConfig `yaml:"services"`
	Routes         []*RouteConfig           `yaml:"routes"`
	RateLimiting   RateLimitingConfig       `yaml:"rate_limiting"`
	JWT            JWTConfig                `yaml:"jwt"`
	AuthService    AuthServiceConfig        `yaml:"auth_service"`
	CircuitBreaker CircuitBreakerConfig     `yaml:"circuit_breaker"`
	Admin          AdminConfig              `yaml:"admin"`
	HotReload      HotReloadConfig          `yaml:"hot_reload"`
}

// ServiceConfig 定义了一个可被路由的上游服务
type ServiceConfig struct {
	Name            string           `yaml:"name"`
	Instances       []InstanceConfig `yaml:"instances"`
	HealthCheckPath string           `yaml:"health_check_path"`
	LoadBalancer    string           `yaml:"load_balancer"`
}

// RouteConfig 定义了一条路由规则
type RouteConfig struct {
	PathPrefix       string            `yaml:"path_prefix,omitempty"`
	Path             string            `yaml:"path,omitempty"`
	ServiceName      string            `yaml:"service_name"`
	Plugins          []PluginSpec      `yaml:"plugins,omitempty"`
	Methods          []string          `yaml:"methods,omitempty"`
	RequiresAuth     bool              `yaml:"requires_auth,omitempty"`
	HealthCheckScope string            `yaml:"health_check_scope,omitempty"`
	UpstreamProtocol string            `yaml:"upstream_protocol,omitempty"` // http/grpc
	ProtocolConvert  string            `yaml:"protocol_convert,omitempty"`  // none/http_json_to_grpc/grpc_to_http_json
	GRPCMethod       string            `yaml:"grpc_method,omitempty"`       // /package.Service/Method
	ProtoDescriptor  string            `yaml:"proto_descriptor_path,omitempty"`
	EmitUnpopulated  bool              `yaml:"emit_unpopulated,omitempty"`
	UseProtoNames    bool              `yaml:"use_proto_names,omitempty"`
	DiscardUnknown   bool              `yaml:"discard_unknown,omitempty"`
	HashOn           string            `yaml:"hash_on,omitempty"` // ip/path/header:<name>/query:<name>
	ABHeader         string            `yaml:"ab_header,omitempty"`
	ABVariants       map[string]string `yaml:"ab_variants,omitempty"`     // header_value -> service
	TrafficWeights   map[string]int    `yaml:"traffic_weights,omitempty"` // service -> weight
}

// ServerConfig 定义服务器配置
type ServerConfig struct {
	Port string `yaml:"port"`
}

// HealthCheckConfig 定义健康检查配置
type HealthCheckConfig struct {
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
}

// InstanceConfig 定义服务实例配置
type InstanceConfig struct {
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
}

// PluginSpec 定义插件配置
type PluginSpec map[string]interface{}

// RateLimitingConfig 定义限流配置
type RateLimitingConfig struct {
	Rules []RateLimiterRule `yaml:"rules"`
}

// RateLimiterRule 定义限流规则
type RateLimiterRule struct {
	Name        string              `yaml:"name"`
	Type        string              `yaml:"type"`
	TokenBucket TokenBucketSettings `yaml:"tokenBucket,omitempty"`
}

// TokenBucketSettings 定义令牌桶设置
type TokenBucketSettings struct {
	Capacity   int `yaml:"capacity"`
	RefillRate int `yaml:"refillRate"`
}

// JWTConfig 定义JWT配置
type JWTConfig struct {
	SecretKey       string `yaml:"secret_key"`
	DurationMinutes int    `yaml:"duration_minutes"`
}

// AuthServiceConfig 定义认证服务配置
type AuthServiceConfig struct {
	ValidateURL string `yaml:"validate_url"`
}

// AdminConfig 定义网关管理面配置。
type AdminConfig struct {
	Token string `yaml:"token"`
}

// HotReloadConfig 定义配置热更新策略。
type HotReloadConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Interval time.Duration `yaml:"interval"`
}

// CircuitBreakerConfig 定义断路器配置
type CircuitBreakerConfig struct {
	FailureThreshold int           `yaml:"failure_threshold"`
	SuccessThreshold int           `yaml:"success_threshold"`
	ResetTimeout     time.Duration `yaml:"reset_timeout"`
}

// Load 从指定路径加载配置文件
func Load(path string) (*GatewayConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 '%s' 失败: %w", path, err)
	}

	var config GatewayConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件 '%s' 失败: %w", path, err)
	}

	return &config, nil
}

// Clone 返回 GatewayConfig 的深拷贝，用于并发安全更新。
func (c *GatewayConfig) Clone() *GatewayConfig {
	if c == nil {
		return nil
	}

	out := *c

	if c.Services != nil {
		out.Services = make(map[string]ServiceConfig, len(c.Services))
		for k, v := range c.Services {
			cloned := v
			if v.Instances != nil {
				cloned.Instances = append([]InstanceConfig(nil), v.Instances...)
			}
			out.Services[k] = cloned
		}
	}

	if c.Routes != nil {
		out.Routes = make([]*RouteConfig, 0, len(c.Routes))
		for _, r := range c.Routes {
			if r == nil {
				out.Routes = append(out.Routes, nil)
				continue
			}
			cloned := *r
			if r.Methods != nil {
				cloned.Methods = append([]string(nil), r.Methods...)
			}
			if r.Plugins != nil {
				cloned.Plugins = make([]PluginSpec, len(r.Plugins))
				copy(cloned.Plugins, r.Plugins)
			}
			if r.ABVariants != nil {
				cloned.ABVariants = make(map[string]string, len(r.ABVariants))
				for k, v := range r.ABVariants {
					cloned.ABVariants[k] = v
				}
			}
			if r.TrafficWeights != nil {
				cloned.TrafficWeights = make(map[string]int, len(r.TrafficWeights))
				for k, v := range r.TrafficWeights {
					cloned.TrafficWeights[k] = v
				}
			}
			out.Routes = append(out.Routes, &cloned)
		}
	}

	if c.RateLimiting.Rules != nil {
		out.RateLimiting.Rules = append([]RateLimiterRule(nil), c.RateLimiting.Rules...)
	}

	return &out
}

