package rbac

import (
	"fmt"
	"net/http"
	"strings"

	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/plugin/httperr"
	"yuelaiengine/gateway/pkg/logger"
)

const PluginName = "rbac"

type Plugin struct {
	logger logger.Logger
}

func NewPlugin(log logger.Logger) *Plugin {
	return &Plugin{logger: log}
}

func (p *Plugin) Name() string {
	return PluginName
}

func (p *Plugin) Execute(w http.ResponseWriter, r *http.Request, pluginCfg config.PluginSpec) (bool, error) {
	headerName, allowedRoles, err := p.parseConfig(pluginCfg)
	if err != nil {
		httperr.Write(w, http.StatusInternalServerError, "PLUGIN_CONFIG_INVALID", "RBAC 插件配置错误")
		return false, fmt.Errorf("[插件 %s] %w", p.Name(), err)
	}

	role := strings.TrimSpace(r.Header.Get(headerName))
	if role == "" {
		httperr.Write(w, http.StatusForbidden, "RBAC_ROLE_MISSING", "缺少角色信息")
		return false, nil
	}

	if _, ok := allowedRoles[strings.ToLower(role)]; !ok {
		httperr.Write(w, http.StatusForbidden, "RBAC_FORBIDDEN", "无权限访问该资源")
		return false, nil
	}

	return true, nil
}

func (p *Plugin) parseConfig(cfg config.PluginSpec) (string, map[string]struct{}, error) {
	headerName, _ := cfg["header"].(string)
	headerName = strings.TrimSpace(headerName)
	if headerName == "" {
		headerName = "X-User-Role"
	}

	rawRoles, ok := cfg["roles"]
	if !ok {
		return "", nil, fmt.Errorf("配置 'roles' 缺失")
	}

	roles := make(map[string]struct{})
	switch v := rawRoles.(type) {
	case []string:
		for _, role := range v {
			role = strings.ToLower(strings.TrimSpace(role))
			if role != "" {
				roles[role] = struct{}{}
			}
		}
	case []interface{}:
		for _, item := range v {
			role, ok := item.(string)
			if !ok {
				continue
			}
			role = strings.ToLower(strings.TrimSpace(role))
			if role != "" {
				roles[role] = struct{}{}
			}
		}
	default:
		return "", nil, fmt.Errorf("配置 'roles' 类型不正确")
	}

	if len(roles) == 0 {
		return "", nil, fmt.Errorf("配置 'roles' 不能为空")
	}

	return headerName, roles, nil
}
