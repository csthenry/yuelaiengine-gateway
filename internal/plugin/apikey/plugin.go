package apikey

import (
	"fmt"
	"net/http"
	"strings"

	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/plugin/httperr"
	"yuelaiengine/gateway/pkg/logger"
)

const PluginName = "apikey"

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
	headerName, keys, err := p.parseConfig(pluginCfg)
	if err != nil {
		httperr.Write(w, http.StatusInternalServerError, "PLUGIN_CONFIG_INVALID", "API Key 插件配置错误")
		return false, fmt.Errorf("[插件 %s] %w", p.Name(), err)
	}

	apiKey := strings.TrimSpace(r.Header.Get(headerName))
	if apiKey == "" {
		httperr.Write(w, http.StatusUnauthorized, "API_KEY_MISSING", "缺少 API Key")
		return false, nil
	}

	if _, ok := keys[apiKey]; !ok {
		httperr.Write(w, http.StatusUnauthorized, "API_KEY_INVALID", "API Key 无效")
		return false, nil
	}

	return true, nil
}

func (p *Plugin) parseConfig(cfg config.PluginSpec) (string, map[string]struct{}, error) {
	headerName, _ := cfg["header"].(string)
	headerName = strings.TrimSpace(headerName)
	if headerName == "" {
		headerName = "X-API-Key"
	}

	rawKeys, ok := cfg["keys"]
	if !ok {
		return "", nil, fmt.Errorf("配置 'keys' 缺失")
	}

	keys := make(map[string]struct{})
	switch v := rawKeys.(type) {
	case []string:
		for _, k := range v {
			k = strings.TrimSpace(k)
			if k != "" {
				keys[k] = struct{}{}
			}
		}
	case []interface{}:
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				keys[s] = struct{}{}
			}
		}
	default:
		return "", nil, fmt.Errorf("配置 'keys' 类型不正确")
	}

	if len(keys) == 0 {
		return "", nil, fmt.Errorf("配置 'keys' 不能为空")
	}

	return headerName, keys, nil
}
