/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-30 21:06:01
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-30 21:34:17
 * @FilePath: /yuelaiengine-gateway/internal/plugin/ratelimit/plugin.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package ratelimit

import (
	"context"
	"fmt"
	"net/http"

	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/core/limiter"
	svc_ratelimit "yuelaiengine/gateway/internal/service/ratelimit"
	"yuelaiengine/gateway/pkg/logger"
)

const (
	PluginName = "ratelimit"
)

// Plugin 实现了 plugin.Interface 接口
type Plugin struct {
	rateLimitSvc svc_ratelimit.Service
	logger       logger.Logger
}

// NewPlugin 创建一个新的限流插件实例
func NewPlugin(svc svc_ratelimit.Service, log logger.Logger) *Plugin {
	if svc == nil {
		log.Fatal(context.Background(), fmt.Sprintf("[插件 %s] 致命错误: ratelimit.Service 依赖注入失败，为 nil", PluginName))
	}
	return &Plugin{
		rateLimitSvc: svc,
		logger:       log,
	}
}

// Name 返回插件的名称
func (p *Plugin) Name() string {
	return PluginName
}

// Execute 执行插件的核心逻辑
func (p *Plugin) Execute(w http.ResponseWriter, r *http.Request, pluginCfg config.PluginSpec) (bool, error) {
	ctx := r.Context()

	// 1. 解析插件配置
	ruleName, strategy, err := p.parseConfig(pluginCfg)
	if err != nil {
		http.Error(w, "限流插件配置错误", http.StatusInternalServerError)
		return false, fmt.Errorf("[插件 %s] %w", p.Name(), err)
	}

	// 2. 根据策略提取标识符
	identifierFunc, err := limiter.GetIdentifierFunc(strategy)
	if err != nil {
		http.Error(w, "限流插件配置错误", http.StatusInternalServerError)
		return false, fmt.Errorf("[插件 %s] %w", p.Name(), err)
	}

	identifier := identifierFunc(r)
	if identifier == "" {
		p.logger.Warn(ctx, "[插件 %s] 警告: 未能根据策略 '%s' 找到有效的请求标识符",
			p.Name(), strategy,
			"plugin", p.Name(),
			"strategy", strategy)
		// 如果无法识别，可以选择放行或拒绝，这里选择放行并记录日志
		return true, nil
	}

	// 3. 使用新的 Service 接口进行限流检查
	allowed, err := p.rateLimitSvc.CheckLimit(ctx, ruleName, identifier)
	if err != nil {
		http.Error(w, "限流服务内部错误", http.StatusInternalServerError)
		return false, fmt.Errorf("[插件 %s] 调用限流服务失败: %w", p.Name(), err)
	}

	if !allowed {
		p.logger.Info(ctx, "[插件 %s] 请求被拒绝: [规则: %s, 标识: %s]",
			p.Name(), ruleName, identifier,
			"plugin", p.Name(),
			"rule", ruleName,
			"identifier", identifier,
			"action", "rejected")
		http.Error(w, "请求过于频繁", http.StatusTooManyRequests)
		return false, nil // 中断插件链
	}

	// 请求被允许，不打印日志，避免日志泛滥
	return true, nil // 继续下一个插件
}

// parseConfig 从配置中解析出规则名称和策略
func (p *Plugin) parseConfig(cfg config.PluginSpec) (string, string, error) {
	rule, ok := cfg["rule"].(string)
	if !ok || rule == "" {
		return "", "", fmt.Errorf("配置 'rule' 缺失或类型不正确")
	}

	strategy, ok := cfg["strategy"].(string)
	if !ok || strategy == "" {
		return "", "", fmt.Errorf("配置 'strategy' 缺失或类型不正确")
	}

	return rule, strategy, nil
}
