/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-24 16:55:20
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-25 19:39:08
 * @FilePath: /yuelaiengine-gateway/internal/core/health/checker.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package health

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
	"yuelaiengine/gateway/pkg/logger"
)

// HealthChecker 负责监控所有上游服务的健康状态
type HealthChecker struct {
	client      *http.Client
	services    sync.Map
	stopChan    chan struct{}
	checkTicker *time.Ticker
	hookMu      sync.RWMutex
	statusHook  func(serviceName, instanceURL string, isHealthy bool)
	logger      logger.Logger
}

// ServiceCheckInfo 存储单个服务的健康信息
type ServiceCheckInfo struct {
	Instances   []string
	HealthPath  string
	Status      map[string]bool // Instance URL -> isHealthy
	statusMutex sync.RWMutex
}

// NewHealthChecker 创建一个新的检查器实例
func NewHealthChecker(timeout time.Duration, interval time.Duration, logger logger.Logger) *HealthChecker {
	return &HealthChecker{
		client: &http.Client{
			Timeout: timeout,
		},
		stopChan:    make(chan struct{}),
		checkTicker: time.NewTicker(interval),
		logger:      logger,
	}
}

// RegisterService 注册服务可进行健康检查
func (hc *HealthChecker) RegisterService(name string, instances []string, healthPath string) {
	statusMap := make(map[string]bool)
	for _, url := range instances {
		statusMap[url] = false // 设置初始状态
	}
	serviceInfo := &ServiceCheckInfo{
		Instances:  instances,
		HealthPath: healthPath,
		Status:     statusMap,
	}
	hc.services.Store(name, serviceInfo)
	hc.logger.Info(context.Background(),
		"[HealthChecker] 服务已注册",
		"service", name,
		"instance count", len(instances),
		"health path", healthPath)
}

// RemoveService 从检查器中移除服务
func (hc *HealthChecker) RemoveService(name string) {
	hc.services.Delete(name)

	hc.logger.Info(context.Background(),
		"[HealthChecker] 服务已移除",
		"service", name)
}

// ListServices 返回已经注册的服务列表
func (hc *HealthChecker) ListServices() []string {
	services := make([]string, 0)
	// 自行查看 iter 包迭代器的使用，可以使其支持 for-range
	// 这里暂时没有处理 for-range 的支持
	hc.services.Range(func(key, val interface{}) bool {
		// 返回包含服务名称的切片
		services = append(services, key.(string))
		return true
	})
	return services
}

// Start 在 goroutine 中启动周期性健康检查
func (hc *HealthChecker) Start() {
	hc.logger.Info(context.Background(), "[HealthChecker] 开启健康检查...")
	for {
		select {
		case <-hc.checkTicker.C:
			hc.runAllHealthChecks()
		case <-hc.stopChan:
			hc.checkTicker.Stop()
			hc.logger.Info(context.Background(), "[HealthChecker] 健康检查已停止")
			return
		}
	}
}

// Shutdown 停止健康检查
func (hc *HealthChecker) Shutdown() {
	close(hc.stopChan)
}

// runAllHealthChecks 遍历所有注册服务并执行并发检查
func (hc *HealthChecker) runAllHealthChecks() {
	ctx := context.Background()
	var wg sync.WaitGroup

	hc.services.Range(func(key, val interface{}) bool {
		name := key.(string)
		info := val.(*ServiceCheckInfo)

		wg.Add(1)
		go func(name string, info *ServiceCheckInfo) {
			defer wg.Done()
			hc.checkService(ctx, name, info)
		}(name, info)
		return true
	})
	wg.Wait()
}

// checkService 检查对应服务
// 此方法会更新 info *ServiceCheckInfo
func (hc *HealthChecker) checkService(ctx context.Context, name string, info *ServiceCheckInfo) {
	for _, url := range info.Instances {
		checkUrl := url + info.HealthPath
		resp, err := hc.client.Get(checkUrl)

		isHealthy := err == nil && resp.StatusCode == http.StatusOK
		if err == nil {
			resp.Body.Close()
		}
		hc.updateServiceStatus(ctx, name, info, url, isHealthy)
	}
}

// SetStatusChangeHook 设置实例健康状态变更回调
func (h *HealthChecker) SetStatusChangeHook(hook func(serviceName, instanceURL string, isHealthy bool)) {
	h.hookMu.Lock()
	defer h.hookMu.Unlock()
	h.statusHook = hook
}

// updateServiceStatus 更新服务状态
func (hc *HealthChecker) updateServiceStatus(ctx context.Context, name string, info *ServiceCheckInfo, url string, isHealthy bool) {
	info.statusMutex.Lock()
	wasHealthy, exists := info.Status[url]
	if exists && wasHealthy == isHealthy {
		info.statusMutex.Unlock()
		return
	}
	info.Status[url] = isHealthy
	info.statusMutex.Unlock()

	statusStr := "healthy"
	if !isHealthy {
		statusStr = "unhealthy"
	}
	hc.logger.Info(ctx, fmt.Sprintf("[HealthChecker] 状态变更 -> 服务: %s, 实例: %s, 当前状态: %s",
		name, url, statusStr))
	hc.notifyStatusChange(name, url, isHealthy)
}

func (h *HealthChecker) notifyStatusChange(serviceName, instanceURL string, isHealthy bool) {
	h.hookMu.RLock()
	hook := h.statusHook
	h.hookMu.RUnlock()
	if hook != nil {
		hook(serviceName, instanceURL, isHealthy)
	}
}

// IsInstanceHealthy 检查特定实例的健康状态
func (hc *HealthChecker) IsInstanceHealthy(name, url string) bool {
	val, ok := hc.services.Load(name)
	if !ok {
		return false
	}
	info := val.(*ServiceCheckInfo)
	info.statusMutex.RLock()
	defer info.statusMutex.RUnlock()

	isHealthy, exists := info.Status[url]
	return exists && isHealthy
}

// GetAllStatuses 返回所有服务的健康状态 /healthz
// healthz 这个命名最早源于 Google 内部的惯例，在 Google 的基础架构中，许多管理类端点都以 z 结尾
// 避免与普通业务混淆
func (hc *HealthChecker) GetAllStatuses() map[string]map[string]bool {
	statuses := make(map[string]map[string]bool)
	hc.services.Range(func(key, val interface{}) bool {
		name := key.(string)
		info := val.(*ServiceCheckInfo)

		info.statusMutex.RLock()
		defer info.statusMutex.RUnlock()

		instanceStatuses := make(map[string]bool)
		for url, isHealthy := range info.Status {
			instanceStatuses[url] = isHealthy
		}
		statuses[name] = instanceStatuses
		return true
	})
	return statuses
}

// GetServiceStatus 返回单个服务的健康状态
func (hc *HealthChecker) GetServiceStatus(name string) map[string]bool {
	val, ok := hc.services.Load(name)
	if !ok {
		return nil
	}
	info := val.(*ServiceCheckInfo)

	info.statusMutex.RLock()
	defer info.statusMutex.RUnlock()

	instanceStatuses := make(map[string]bool)
	for url, isHealthy := range info.Status {
		instanceStatuses[url] = isHealthy
	}

	return instanceStatuses
}
