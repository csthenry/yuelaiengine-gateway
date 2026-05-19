package core

import (
	"fmt"
	"net/url"
	"strings"

	"yuelaiengine/gateway/internal/config"
)

func validateGatewayConfig(cfg *config.GatewayConfig) error {
	if cfg == nil {
		return fmt.Errorf("配置不能为空")
	}
	if strings.TrimSpace(cfg.Server.Port) == "" {
		return fmt.Errorf("server.port 不能为空")
	}
	if len(cfg.Services) == 0 {
		return fmt.Errorf("services 不能为空")
	}

	for key, svc := range cfg.Services {
		if strings.TrimSpace(svc.Name) == "" {
			return fmt.Errorf("service[%s].name 不能为空", key)
		}
		if len(svc.Instances) == 0 {
			return fmt.Errorf("service[%s].instances 不能为空", svc.Name)
		}
		if strings.TrimSpace(svc.HealthCheckPath) == "" {
			return fmt.Errorf("service[%s].health_check_path 不能为空", svc.Name)
		}
		if !strings.HasPrefix(svc.HealthCheckPath, "/") {
			return fmt.Errorf("service[%s].health_check_path 必须以 / 开头", svc.Name)
		}
		for _, inst := range svc.Instances {
			u, err := url.Parse(inst.URL)
			if err != nil || strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
				return fmt.Errorf("service[%s] 存在非法实例 URL: %s", svc.Name, inst.URL)
			}
		}
	}

	if len(cfg.Routes) == 0 {
		return fmt.Errorf("routes 不能为空")
	}

	for _, route := range cfg.Routes {
		if route == nil {
			return fmt.Errorf("routes 中存在空条目")
		}
		if strings.TrimSpace(route.PathPrefix) == "" && strings.TrimSpace(route.Path) == "" {
			return fmt.Errorf("route 必须配置 path 或 path_prefix")
		}
		if strings.TrimSpace(route.ServiceName) == "" {
			return fmt.Errorf("route(path_prefix=%s) service_name 不能为空", route.PathPrefix)
		}

		if route.ServiceName != "all-services" {
			if _, ok := cfg.Services[route.ServiceName]; !ok {
				return fmt.Errorf("route(path_prefix=%s) 引用的 service_name=%s 未定义", route.PathPrefix, route.ServiceName)
			}
		}

		for svcName, weight := range route.TrafficWeights {
			if _, ok := cfg.Services[svcName]; !ok {
				return fmt.Errorf("route(path_prefix=%s) traffic_weights 引用了未定义服务 %s", route.PathPrefix, svcName)
			}
			if weight <= 0 {
				return fmt.Errorf("route(path_prefix=%s) traffic_weights[%s] 必须 > 0", route.PathPrefix, svcName)
			}
		}

		for _, svcName := range route.ABVariants {
			if _, ok := cfg.Services[svcName]; !ok {
				return fmt.Errorf("route(path_prefix=%s) ab_variants 引用了未定义服务 %s", route.PathPrefix, svcName)
			}
		}

		for _, spec := range route.Plugins {
			if err := validatePluginSpec(route.PathPrefix, spec); err != nil {
				return err
			}
		}
	}

	for _, rule := range cfg.RateLimiting.Rules {
		if strings.TrimSpace(rule.Name) == "" {
			return fmt.Errorf("rate_limiting.rules 中存在空 name")
		}
		if strings.TrimSpace(rule.Type) == "memory_token_bucket" {
			if rule.TokenBucket.Capacity <= 0 {
				return fmt.Errorf("rate_limiting.rules[%s].tokenBucket.capacity 必须 > 0", rule.Name)
			}
			if rule.TokenBucket.RefillRate < 0 {
				return fmt.Errorf("rate_limiting.rules[%s].tokenBucket.refillRate 必须 >= 0", rule.Name)
			}
		}
	}

	if cfg.Monitoring.PersistEnabled {
		if cfg.Monitoring.FlushInterval < 0 {
			return fmt.Errorf("monitoring.flush_interval 不能小于 0")
		}
	}

	return nil
}

func validatePluginSpec(pathPrefix string, spec config.PluginSpec) error {
	name, _ := spec["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("route(path_prefix=%s) 插件缺少 name", pathPrefix)
	}

	switch name {
	case "ratelimit":
		if strings.TrimSpace(asString(spec["rule"])) == "" {
			return fmt.Errorf("route(path_prefix=%s) ratelimit.rule 不能为空", pathPrefix)
		}
		if strings.TrimSpace(asString(spec["strategy"])) == "" {
			return fmt.Errorf("route(path_prefix=%s) ratelimit.strategy 不能为空", pathPrefix)
		}
	case "circuitbreaker":
		if strings.TrimSpace(asString(spec["service"])) == "" {
			return fmt.Errorf("route(path_prefix=%s) circuitbreaker.service 不能为空", pathPrefix)
		}
	case "apikey":
		if _, ok := spec["keys"]; !ok {
			return fmt.Errorf("route(path_prefix=%s) apikey.keys 不能为空", pathPrefix)
		}
	case "rbac":
		if _, ok := spec["roles"]; !ok {
			return fmt.Errorf("route(path_prefix=%s) rbac.roles 不能为空", pathPrefix)
		}
	}

	return nil
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}
