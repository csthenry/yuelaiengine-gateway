/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-30 21:35:48
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-31 19:47:37
 * @FilePath: /yuelaiengine-gateway/internal/plugin/manager.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package plugin

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/plugin/httperr"
	"yuelaiengine/gateway/pkg/logger"
)

// Interface 定义了插件必须实现的接口
type Interface interface {
	Name() string
	Execute(w http.ResponseWriter, r *http.Request, params config.PluginSpec) (continueChain bool, err error)
}

// Manager 负责管理和执行插件
type Manager struct {
	plugins map[string]Interface
	mu      sync.RWMutex
	log     logger.Logger
}

func NewManager(log logger.Logger) *Manager {
	if log == nil {
		var err error
		log, err = logger.DefaultNew()
		if err != nil {
			// 如果无法初始化日志记录器，使用标准输出输出错误信息并退出
			panic(fmt.Sprintf("[插件管理器] 无法初始化日志记录器: %v", err))
		}
	}

	return &Manager{
		plugins: make(map[string]Interface),
		log:     log,
	}
}

func (m *Manager) GetPlugin(name string) Interface {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.plugins[name]
}

// Register 注册一个插件
func (m *Manager) Register(p Interface) {
	ctx := context.Background()
	name := p.Name()

	m.log.Info(ctx, fmt.Sprintf("[插件管理器] 正在注册插件 '%s'", name),
		"plugin_name", name,
		"action", "register")

	if _, exists := m.plugins[name]; exists {
		m.log.Warn(ctx, fmt.Sprintf("[插件管理器] 警告: 插件 '%s' 已存在，将被覆盖", name),
			"plugin_name", name,
			"action", "overwrite")
	}
	m.mu.Lock()
	m.plugins[name] = p
	m.mu.Unlock()
}

// ExecuteChain 执行插件链
func (m *Manager) ExecuteChain(w http.ResponseWriter, r *http.Request, pluginSpecs []config.PluginSpec) (bool, error) {
	ctx := r.Context()

	for _, spec := range pluginSpecs {
		pluginName, ok := spec["name"].(string)
		if !ok || pluginName == "" {
			m.log.Error(ctx, fmt.Sprintf("[插件管理器] 插件配置缺少 'name' 字段或类型不正确: %v", spec),
				"spec", spec,
				"action", "config_error")
			httperr.Write(w, http.StatusInternalServerError, "PLUGIN_CONFIG_INVALID", "插件配置错误")
			return false, fmt.Errorf("无效的插件配置: %v", spec)
		}

		plugin := m.GetPlugin(pluginName)
		if plugin == nil {
			m.log.Error(ctx, fmt.Sprintf("[插件管理器] 未找到名为 '%s' 的已注册插件", pluginName),
				"plugin_name", pluginName,
				"action", "plugin_not_found")
			httperr.Write(w, http.StatusInternalServerError, "PLUGIN_NOT_FOUND", "插件未找到")
			return false, fmt.Errorf("插件 '%s' 未注册", pluginName)
		}

		m.log.Debug(ctx, fmt.Sprintf("[插件管理器] 执行插件: %s", pluginName),
			"plugin_name", pluginName,
			"action", "execute")

		continueChain, err := plugin.Execute(w, r, spec)
		if err != nil {
			m.log.Error(ctx, fmt.Sprintf("[插件管理器] 插件 '%s' 执行时返回内部错误: %v", pluginName, err),
				"plugin_name", pluginName,
				"error", err.Error(),
				"action", "execute_error")
			return false, err
		}

		if !continueChain {
			m.log.Debug(ctx, fmt.Sprintf("[插件管理器] 插件 '%s' 中断了请求链", pluginName),
				"plugin_name", pluginName,
				"action", "chain_interrupted")
			return false, nil
		}
	}

	return true, nil
}
