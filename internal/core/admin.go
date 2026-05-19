package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/plugin/httperr"
)

type adminResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// HandleMetricsHTTP 暴露给路由层的指标入口。
func (g *Gateway) HandleMetricsHTTP(w http.ResponseWriter, r *http.Request) {
	g.handleMetrics(w, r)
}

// HandleWebUIHTTP 暴露给路由层的 Web 控制台入口。
func (g *Gateway) HandleWebUIHTTP(w http.ResponseWriter, r *http.Request) {
	g.handleWebUI(w, r)
}

func (g *Gateway) adminEndpoint(w http.ResponseWriter, r *http.Request, method string, handler func(http.ResponseWriter, *http.Request)) {
	if !g.authorizeAdmin(r) {
		httperr.Write(w, http.StatusUnauthorized, "ADMIN_UNAUTHORIZED", "管理接口鉴权失败")
		return
	}
	if method != "" && r.Method != method {
		httperr.Write(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "仅支持 "+method)
		return
	}
	handler(w, r)
}

func (g *Gateway) AdminCircuitStatusHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodGet, g.handleAdminCircuitStatus)
}

func (g *Gateway) AdminCircuitResetHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodPost, g.handleAdminCircuitReset)
}

func (g *Gateway) AdminRateLimitRuleListHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodGet, g.handleAdminRateLimitRuleList)
}

func (g *Gateway) AdminRateLimitRuleUpsertHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodPost, g.handleAdminRateLimitRuleUpsert)
}

func (g *Gateway) AdminRateLimitRuleDeleteHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodPost, g.handleAdminRateLimitRuleDelete)
}

func (g *Gateway) AdminRouteListHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodGet, g.handleAdminRouteList)
}

func (g *Gateway) AdminRouteUpsertHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodPost, g.handleAdminRouteUpsert)
}

func (g *Gateway) AdminRouteDeleteHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodPost, g.handleAdminRouteDelete)
}

func (g *Gateway) AdminServiceListHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodGet, g.handleAdminServiceList)
}

func (g *Gateway) AdminServiceUpsertHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodPost, g.handleAdminServiceUpsert)
}

func (g *Gateway) AdminServiceDeleteHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodPost, g.handleAdminServiceDelete)
}

func (g *Gateway) AdminConfigVersionsHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodGet, g.handleAdminConfigVersions)
}

func (g *Gateway) AdminConfigCurrentHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodGet, g.handleAdminConfigCurrent)
}

func (g *Gateway) AdminConfigRollbackHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodPost, g.handleAdminConfigRollback)
}

func (g *Gateway) AdminConfigApplyHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodPost, g.handleAdminConfigApply)
}

func (g *Gateway) AdminHealthStatusHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodGet, g.handleAdminHealthStatus)
}

func (g *Gateway) AdminMetricsSummaryHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodGet, g.handleAdminMetricsSummary)
}

func (g *Gateway) AdminMetricsHistoryHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, http.MethodGet, g.handleAdminMetricsHistory)
}

func (g *Gateway) AdminNotFoundHTTP(w http.ResponseWriter, r *http.Request) {
	g.adminEndpoint(w, r, "", func(w http.ResponseWriter, _ *http.Request) {
		httperr.Write(w, http.StatusNotFound, "ADMIN_API_NOT_FOUND", "管理接口不存在")
	})
}

func (g *Gateway) authorizeAdmin(r *http.Request) bool {
	cfg, _ := g.snapshot()
	expected := ""
	if cfg != nil {
		expected = strings.TrimSpace(cfg.Admin.Token)
	}
	if expected == "" {
		return true
	}
	if strings.TrimSpace(r.Header.Get("X-Admin-Token")) == expected {
		return true
	}
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if authz == "" {
		return false
	}
	parts := strings.SplitN(authz, " ", 2)
	if len(parts) != 2 {
		return false
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return false
	}
	return strings.TrimSpace(parts[1]) == expected
}

func (g *Gateway) handleAdminCircuitStatus(w http.ResponseWriter, r *http.Request) {
	if g.circuitBreakerSvc == nil {
		httperr.Write(w, http.StatusServiceUnavailable, "CIRCUIT_BREAKER_NOT_READY", "熔断服务未初始化")
		return
	}

	writeJSON(w, http.StatusOK, adminResponse{
		Status: "ok",
		Data: map[string]interface{}{
			"circuits": g.circuitBreakerSvc.GetAllState(r.Context()),
		},
	})
}

func (g *Gateway) handleAdminCircuitReset(w http.ResponseWriter, r *http.Request) {
	if g.circuitBreakerSvc == nil {
		httperr.Write(w, http.StatusServiceUnavailable, "CIRCUIT_BREAKER_NOT_READY", "熔断服务未初始化")
		return
	}

	serviceName := strings.TrimSpace(r.URL.Query().Get("service"))
	if serviceName == "" {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "缺少 service 参数")
		return
	}
	if err := g.circuitBreakerSvc.Reset(r.Context(), serviceName); err != nil {
		httperr.Write(w, http.StatusInternalServerError, "CIRCUIT_RESET_FAILED", "重置熔断器失败")
		return
	}

	writeJSON(w, http.StatusOK, adminResponse{
		Status:  "ok",
		Message: "熔断器重置成功",
		Data: map[string]string{
			"service": serviceName,
		},
	})
}

func (g *Gateway) handleAdminRateLimitRuleList(w http.ResponseWriter, r *http.Request) {
	cfg, _ := g.snapshot()
	if cfg == nil {
		httperr.Write(w, http.StatusServiceUnavailable, "CONFIG_NOT_READY", "网关配置未就绪")
		return
	}
	writeJSON(w, http.StatusOK, adminResponse{
		Status: "ok",
		Data:   rateLimitRuleDTOListFromConfig(cfg.RateLimiting.Rules),
	})
}

type rateLimitUpsertRequest struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Capacity   int    `json:"capacity"`
	RefillRate int    `json:"refill_rate"`
}

func (g *Gateway) handleAdminRateLimitRuleUpsert(w http.ResponseWriter, r *http.Request) {
	var req rateLimitUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "请求体格式错误")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Type = strings.TrimSpace(req.Type)
	if req.Name == "" {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "name 不能为空")
		return
	}
	if req.Type == "" {
		req.Type = "memory_token_bucket"
	}

	err := g.mutateConfig("admin:ratelimit:upsert", func(next *config.GatewayConfig) error {
		rules := next.RateLimiting.Rules
		idx := -1
		for i := range rules {
			if rules[i].Name == req.Name {
				idx = i
				break
			}
		}

		rule := config.RateLimiterRule{
			Name: req.Name,
			Type: req.Type,
			TokenBucket: config.TokenBucketSettings{
				Capacity:   req.Capacity,
				RefillRate: req.RefillRate,
			},
		}

		if idx >= 0 {
			next.RateLimiting.Rules[idx] = rule
		} else {
			next.RateLimiting.Rules = append(next.RateLimiting.Rules, rule)
		}
		return nil
	})
	if err != nil {
		httperr.Write(w, http.StatusBadRequest, "CONFIG_APPLY_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, adminResponse{Status: "ok", Message: "限流规则已更新"})
}

type rateLimitDeleteRequest struct {
	Name string `json:"name"`
}

func (g *Gateway) handleAdminRateLimitRuleDelete(w http.ResponseWriter, r *http.Request) {
	var req rateLimitDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "请求体格式错误")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "name 不能为空")
		return
	}

	err := g.mutateConfig("admin:ratelimit:delete", func(next *config.GatewayConfig) error {
		rules := next.RateLimiting.Rules
		out := make([]config.RateLimiterRule, 0, len(rules))
		removed := false
		for _, rule := range rules {
			if rule.Name == req.Name {
				removed = true
				continue
			}
			out = append(out, rule)
		}
		if !removed {
			return fmt.Errorf("限流规则 %s 不存在", req.Name)
		}
		next.RateLimiting.Rules = out
		return nil
	})
	if err != nil {
		httperr.Write(w, http.StatusBadRequest, "CONFIG_APPLY_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, adminResponse{Status: "ok", Message: "限流规则已删除"})
}

func (g *Gateway) handleAdminRouteList(w http.ResponseWriter, r *http.Request) {
	cfg, _ := g.snapshot()
	if cfg == nil {
		httperr.Write(w, http.StatusServiceUnavailable, "CONFIG_NOT_READY", "网关配置未就绪")
		return
	}
	writeJSON(w, http.StatusOK, adminResponse{
		Status: "ok",
		Data:   routeDTOListFromConfig(cfg.Routes),
	})
}

type routeUpsertRequest struct {
	Route json.RawMessage `json:"route"`
}

func (g *Gateway) handleAdminRouteUpsert(w http.ResponseWriter, r *http.Request) {
	var req routeUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "请求体格式错误")
		return
	}
	route, err := decodeRouteConfig(req.Route)
	if err != nil {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	if strings.TrimSpace(route.PathPrefix) == "" && strings.TrimSpace(route.Path) == "" {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "route.path 或 route.path_prefix 必须提供其一")
		return
	}

	err = g.mutateConfig("admin:routes:upsert", func(next *config.GatewayConfig) error {
		updated := false
		for i, existing := range next.Routes {
			if existing == nil {
				continue
			}
			if sameRouteIdentity(existing, &route) {
				cloned := route
				next.Routes[i] = &cloned
				updated = true
				break
			}
		}
		if !updated {
			cloned := route
			next.Routes = append(next.Routes, &cloned)
		}
		return nil
	})
	if err != nil {
		httperr.Write(w, http.StatusBadRequest, "CONFIG_APPLY_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, adminResponse{Status: "ok", Message: "路由已更新"})
}

type routeDeleteRequest struct {
	PathPrefix string `json:"path_prefix"`
	Path       string `json:"path"`
}

func (g *Gateway) handleAdminRouteDelete(w http.ResponseWriter, r *http.Request) {
	var req routeDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "请求体格式错误")
		return
	}
	req.PathPrefix = strings.TrimSpace(req.PathPrefix)
	req.Path = strings.TrimSpace(req.Path)
	if req.PathPrefix == "" && req.Path == "" {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "path 或 path_prefix 必须提供其一")
		return
	}

	err := g.mutateConfig("admin:routes:delete", func(next *config.GatewayConfig) error {
		out := make([]*config.RouteConfig, 0, len(next.Routes))
		removed := false
		for _, route := range next.Routes {
			if route == nil {
				continue
			}
			if (req.Path != "" && route.Path == req.Path) || (req.PathPrefix != "" && route.PathPrefix == req.PathPrefix) {
				removed = true
				continue
			}
			out = append(out, route)
		}
		if !removed {
			return fmt.Errorf("路由不存在")
		}
		next.Routes = out
		return nil
	})
	if err != nil {
		httperr.Write(w, http.StatusBadRequest, "CONFIG_APPLY_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, adminResponse{Status: "ok", Message: "路由已删除"})
}

func sameRouteIdentity(a, b *config.RouteConfig) bool {
	if a == nil || b == nil {
		return false
	}
	if strings.TrimSpace(a.Path) != "" || strings.TrimSpace(b.Path) != "" {
		return strings.TrimSpace(a.Path) == strings.TrimSpace(b.Path)
	}
	return strings.TrimSpace(a.PathPrefix) == strings.TrimSpace(b.PathPrefix)
}

func decodeRouteConfig(raw json.RawMessage) (config.RouteConfig, error) {
	var route config.RouteConfig
	if len(raw) == 0 {
		return route, fmt.Errorf("route 不能为空")
	}

	// 优先按统一 DTO（snake_case）解码。
	var dto adminRouteDTO
	if err := json.Unmarshal(raw, &dto); err == nil {
		if strings.TrimSpace(dto.PathPrefix) != "" || strings.TrimSpace(dto.Path) != "" {
			return dto.toConfigRoute(), nil
		}
	}

	// 兼容旧请求体（Go 字段名风格）。
	if err := json.Unmarshal(raw, &route); err == nil {
		if strings.TrimSpace(route.PathPrefix) != "" || strings.TrimSpace(route.Path) != "" {
			return route, nil
		}
	}

	return route, fmt.Errorf("route 格式错误")
}

func (g *Gateway) handleAdminServiceList(w http.ResponseWriter, r *http.Request) {
	cfg, _ := g.snapshot()
	if cfg == nil {
		httperr.Write(w, http.StatusServiceUnavailable, "CONFIG_NOT_READY", "网关配置未就绪")
		return
	}
	writeJSON(w, http.StatusOK, adminResponse{
		Status: "ok",
		Data:   serviceDTOListFromConfig(cfg.Services),
	})
}

type serviceUpsertRequest struct {
	Service json.RawMessage `json:"service"`
}

func (g *Gateway) handleAdminServiceUpsert(w http.ResponseWriter, r *http.Request) {
	var req serviceUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "请求体格式错误")
		return
	}

	service, err := decodeServiceConfig(req.Service)
	if err != nil {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	err = g.mutateConfig("admin:services:upsert", func(next *config.GatewayConfig) error {
		if next.Services == nil {
			next.Services = make(map[string]config.ServiceConfig)
		}
		next.Services[service.Name] = service
		return nil
	})
	if err != nil {
		httperr.Write(w, http.StatusBadRequest, "CONFIG_APPLY_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, adminResponse{Status: "ok", Message: "服务配置已更新"})
}

type serviceDeleteRequest struct {
	Name string `json:"name"`
}

func (g *Gateway) handleAdminServiceDelete(w http.ResponseWriter, r *http.Request) {
	var req serviceDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "请求体格式错误")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "name 不能为空")
		return
	}

	err := g.mutateConfig("admin:services:delete", func(next *config.GatewayConfig) error {
		if len(next.Services) == 0 {
			return fmt.Errorf("服务 %s 不存在", req.Name)
		}
		if _, ok := next.Services[req.Name]; !ok {
			return fmt.Errorf("服务 %s 不存在", req.Name)
		}
		delete(next.Services, req.Name)
		return nil
	})
	if err != nil {
		httperr.Write(w, http.StatusBadRequest, "CONFIG_APPLY_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, adminResponse{Status: "ok", Message: "服务配置已删除"})
}

func decodeServiceConfig(raw json.RawMessage) (config.ServiceConfig, error) {
	var service config.ServiceConfig
	if len(raw) == 0 {
		return service, fmt.Errorf("service 不能为空")
	}

	var dto adminServiceDTO
	if err := json.Unmarshal(raw, &dto); err == nil {
		dto.Name = strings.TrimSpace(dto.Name)
		if dto.Name != "" {
			out := dto.toConfigService()
			out.Name = strings.TrimSpace(out.Name)
			if out.Name == "" {
				return out, fmt.Errorf("service.name 不能为空")
			}
			if strings.TrimSpace(out.LoadBalancer) == "" {
				out.LoadBalancer = "round_robin"
			}
			return out, nil
		}
	}

	if err := json.Unmarshal(raw, &service); err != nil {
		return service, fmt.Errorf("service 格式错误")
	}
	service.Name = strings.TrimSpace(service.Name)
	if service.Name == "" {
		return service, fmt.Errorf("service.name 不能为空")
	}
	if strings.TrimSpace(service.LoadBalancer) == "" {
		service.LoadBalancer = "round_robin"
	}
	return service, nil
}

func (g *Gateway) handleAdminConfigVersions(w http.ResponseWriter, r *http.Request) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	writeJSON(w, http.StatusOK, adminResponse{
		Status: "ok",
		Data: map[string]interface{}{
			"current": versionMetaDTO(g.currentConfigVersionLocked()),
			"history": versionMetaDTOList(g.configVersionListLocked()),
		},
	})
}

func (g *Gateway) handleAdminConfigCurrent(w http.ResponseWriter, r *http.Request) {
	cfg, _ := g.snapshot()
	if cfg == nil {
		httperr.Write(w, http.StatusServiceUnavailable, "CONFIG_NOT_READY", "网关配置未就绪")
		return
	}

	g.mu.RLock()
	current := g.currentConfigVersionLocked()
	g.mu.RUnlock()

	writeJSON(w, http.StatusOK, adminResponse{
		Status: "ok",
		Data: map[string]interface{}{
			"version": versionMetaDTO(current),
			"config":  cfg.Clone(),
		},
	})
}

type rollbackRequest struct {
	Version string `json:"version"`
}

func (g *Gateway) handleAdminConfigRollback(w http.ResponseWriter, r *http.Request) {
	var req rollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "请求体格式错误")
		return
	}

	req.Version = strings.TrimSpace(req.Version)
	g.mu.Lock()
	err := g.rollbackToVersionLocked(req.Version)
	g.mu.Unlock()
	if err != nil {
		httperr.Write(w, http.StatusBadRequest, "ROLLBACK_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, adminResponse{Status: "ok", Message: "配置回滚成功"})
}

type configApplyRequest struct {
	Config      config.GatewayConfig `json:"config"`
	DryRun      bool                 `json:"dry_run"`
	Source      string               `json:"source"`
	Persist     bool                 `json:"persist"`
	PersistPath string               `json:"persist_path,omitempty"`
}

func (g *Gateway) handleAdminConfigApply(w http.ResponseWriter, r *http.Request) {
	var req configApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.Write(w, http.StatusBadRequest, "BAD_REQUEST", "请求体格式错误")
		return
	}

	if req.Source == "" {
		req.Source = "admin:config:apply"
	}
	req.PersistPath = strings.TrimSpace(req.PersistPath)

	if req.DryRun {
		if err := validateGatewayConfig(req.Config.Clone()); err != nil {
			httperr.Write(w, http.StatusBadRequest, "CONFIG_VALIDATE_FAILED", err.Error())
			return
		}
		if err := validateRouteTranscodingConfigs(req.Config.Routes); err != nil {
			httperr.Write(w, http.StatusBadRequest, "CONFIG_VALIDATE_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, adminResponse{Status: "ok", Message: "配置校验通过"})
		return
	}

	successMessage := "配置已应用"
	successData := map[string]string(nil)

	g.mu.Lock()
	rollbackSnapshot := g.config.Clone()
	err := g.applyConfigLocked(req.Config.Clone(), req.Source)
	persistPath := ""
	if err == nil && req.Persist {
		persistPath = req.PersistPath
		if persistPath == "" {
			persistPath = strings.TrimSpace(g.reloadPath)
		}
		if persistPath == "" {
			err = fmt.Errorf("persist=true 时未提供 persist_path，且网关未设置默认配置路径")
		} else {
			err = config.Save(persistPath, g.config.Clone())
			if err == nil {
				g.reloadPath = persistPath
				successMessage = "配置已应用并落盘"
				successData = map[string]string{"persist_path": persistPath}
			}
		}
	}
	if err != nil && req.Persist && rollbackSnapshot != nil {
		if rollbackErr := g.applyConfigLocked(rollbackSnapshot, "rollback:persist-failed"); rollbackErr != nil {
			err = fmt.Errorf("发布失败: %v；且回滚内存配置失败: %v", err, rollbackErr)
		} else if persistPath != "" {
			err = fmt.Errorf("配置落盘失败，已回滚内存配置: %v (path=%s)", err, persistPath)
		} else {
			err = fmt.Errorf("配置发布失败，已回滚内存配置: %v", err)
		}
	}
	g.mu.Unlock()
	if err != nil {
		httperr.Write(w, http.StatusBadRequest, "CONFIG_APPLY_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, adminResponse{Status: "ok", Message: successMessage, Data: successData})
}

func (g *Gateway) handleAdminHealthStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, adminResponse{
		Status: "ok",
		Data:   g.healthChecker.GetAllStatuses(),
	})
}

func (g *Gateway) handleAdminMetricsSummary(w http.ResponseWriter, r *http.Request) {
	var openCount uint64
	if g.circuitBreakerSvc != nil {
		openCount = g.circuitBreakerSvc.OpenTransitionCount(r.Context())
	}

	writeJSON(w, http.StatusOK, adminResponse{
		Status: "ok",
		Data:   g.metrics.Snapshot(openCount),
	})
}

func (g *Gateway) handleAdminMetricsHistory(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	g.mu.RLock()
	path := g.monitorPath
	g.mu.RUnlock()
	if strings.TrimSpace(path) == "" {
		path = "./logs/monitoring/monitor-history.jsonl"
	}

	records, err := readMonitorHistory(path, limit)
	if err != nil {
		httperr.Write(w, http.StatusInternalServerError, "METRICS_HISTORY_READ_FAILED", "读取监控历史失败")
		return
	}

	writeJSON(w, http.StatusOK, adminResponse{
		Status: "ok",
		Data: map[string]interface{}{
			"path":    path,
			"records": records,
		},
	})
}

func (g *Gateway) mutateConfig(source string, mutator func(next *config.GatewayConfig) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.config == nil {
		return fmt.Errorf("当前配置不可用")
	}
	next := g.config.Clone()
	if err := mutator(next); err != nil {
		return err
	}
	return g.applyConfigLocked(next, source)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (g *Gateway) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httperr.Write(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "仅支持 GET")
		return
	}

	var openCount uint64
	if g.circuitBreakerSvc != nil {
		openCount = g.circuitBreakerSvc.OpenTransitionCount(r.Context())
	}
	payload := g.metrics.RenderPrometheus(openCount)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(payload))
}
