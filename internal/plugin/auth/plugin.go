package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/core/health"
	"yuelaiengine/gateway/internal/core/loadbalancer"
	"yuelaiengine/gateway/internal/plugin/httperr"
	"yuelaiengine/gateway/pkg/logger"
)

const PluginName = "auth"

type Plugin struct {
	lbFactory      *loadbalancer.LoadBalancerFactory
	healthChecker  *health.HealthChecker
	authService    string
	validatePath   string
	httpClient     *http.Client
	logger         logger.Logger
}

type validateRequest struct {
	Token string `json:"token"`
}

type validateResponse struct {
	Valid  bool   `json:"valid"`
	UserID string `json:"user_id,omitempty"`
	Role   string `json:"role,omitempty"`
}

func NewPlugin(lbFactory *loadbalancer.LoadBalancerFactory, hc *health.HealthChecker, serviceName, validateURL string, log logger.Logger) (*Plugin, error) {
	if lbFactory == nil {
		return nil, errors.New("loadbalancer factory 不能为空")
	}
	if strings.TrimSpace(serviceName) == "" {
		return nil, errors.New("auth service name 不能为空")
	}

	path, err := parseValidatePath(validateURL)
	if err != nil {
		return nil, err
	}

	return &Plugin{
		lbFactory:     lbFactory,
		healthChecker: hc,
		authService:   strings.TrimSpace(serviceName),
		validatePath:  path,
		httpClient: &http.Client{Timeout: 3 * time.Second},
		logger:        log,
	}, nil
}

func (p *Plugin) Name() string {
	return PluginName
}

func (p *Plugin) Execute(w http.ResponseWriter, r *http.Request, pluginCfg config.PluginSpec) (bool, error) {
	token := extractToken(r)
	if token == "" {
		httperr.Write(w, http.StatusUnauthorized, "AUTH_TOKEN_MISSING", "缺少访问令牌")
		return false, nil
	}

	instance, err := p.pickInstance()
	if err != nil {
		httperr.Write(w, http.StatusBadGateway, "AUTH_SERVICE_UNAVAILABLE", "认证服务不可用")
		return false, fmt.Errorf("[插件 %s] 选择认证服务实例失败: %w", p.Name(), err)
	}

	validateURL := strings.TrimRight(instance.URL, "/") + p.validatePath
	result, err := p.callValidate(validateURL, token)
	if err != nil {
		httperr.Write(w, http.StatusBadGateway, "AUTH_VALIDATE_FAILED", "认证服务校验失败")
		return false, fmt.Errorf("[插件 %s] 调用认证服务失败: %w", p.Name(), err)
	}

	if !result.Valid {
		httperr.Write(w, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "令牌无效或已过期")
		return false, nil
	}

	if result.UserID != "" {
		r.Header.Set("X-User-ID", result.UserID)
	}
	if result.Role != "" {
		r.Header.Set("X-User-Role", result.Role)
	}

	return true, nil
}

func parseValidatePath(validateURL string) (string, error) {
	trimmed := strings.TrimSpace(validateURL)
	if trimmed == "" {
		return "/validate", nil
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("validate_url 非法: %w", err)
	}
	if u.Path == "" {
		return "/validate", nil
	}
	if strings.HasPrefix(u.Path, "/") {
		return u.Path, nil
	}
	return "/" + u.Path, nil
}

func extractToken(r *http.Request) string {
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if authz != "" {
		parts := strings.SplitN(authz, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	return strings.TrimSpace(r.Header.Get("X-Auth-Token"))
}

func (p *Plugin) pickInstance() (*loadbalancer.ServiceInstance, error) {
	lb := p.lbFactory.GetOrCreateLoadBalancer(p.authService, "round_robin")
	instances := lb.GetAllInstances(p.authService)
	if len(instances) == 0 {
		return nil, errors.New("认证服务无可用实例")
	}

	for i := 0; i < len(instances); i++ {
		inst, err := lb.GetNextInstance(p.authService)
		if err != nil {
			return nil, err
		}
		if p.healthChecker == nil || p.healthChecker.IsInstanceHealthy(p.authService, inst.URL) {
			return inst, nil
		}
	}

	// 启动初期健康检查还未完成时，回退到第一个实例，避免冷启动误拒绝。
	return instances[0], nil
}

func (p *Plugin) callValidate(validateURL, token string) (*validateResponse, error) {
	payload, err := json.Marshal(validateRequest{Token: token})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, validateURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return &validateResponse{Valid: false}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("认证服务返回状态码 %d", resp.StatusCode)
	}

	var result validateResponse
	if len(body) == 0 {
		return nil, errors.New("认证服务返回空响应")
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析认证服务响应失败: %w", err)
	}
	return &result, nil
}
