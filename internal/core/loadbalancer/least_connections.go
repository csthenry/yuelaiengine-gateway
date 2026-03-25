/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-25 19:51:30
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-25 20:01:07
 * @FilePath: /yuelaiengine-gateway/internal/core/loadbalancer/least_connections.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package loadbalancer

import (
	"errors"
	"fmt"
	"sync"
)

type LeastConnectionsBalancer struct {
	serviceName string
	instances   []*ServiceInstance
	mutex       sync.RWMutex
}

func NewLeastConnectionsBalancer(serviceName string) *LeastConnectionsBalancer {
	return &LeastConnectionsBalancer{
		serviceName: serviceName,
	}
}

func (l *LeastConnectionsBalancer) RegisterInstance(serviceName string, instance *ServiceInstance) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// 连接数由运行时动态维护
	l.instances = append(l.instances, instance)
}

func (l *LeastConnectionsBalancer) GetNextInstance(serviceName string) (*ServiceInstance, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if len(l.instances) == 0 {
		return nil, errors.New("no instances available")
	}

	// 过滤健康实例
	healthyInstances := make([]*ServiceInstance, 0)
	for _, instance := range l.instances {
		if instance.Alive {
			healthyInstances = append(healthyInstances, instance)
		}
	}

	if len(healthyInstances) == 0 {
		return nil, errors.New("no instances available")
	}

	// 遍历选择连接数最少的健康实例
	minConnections := healthyInstances[0].Connections
	selectedInstance := healthyInstances[0]

	for _, instance := range healthyInstances {
		if instance.Connections < minConnections {
			minConnections = instance.Connections
			selectedInstance = instance
		}
	}

	// 命中实例后，请求数 +1，请求结束时 -1
	selectedInstance.Connections++
	return selectedInstance, nil
}

func (l *LeastConnectionsBalancer) GetAllInstances(serviceName string) []*ServiceInstance {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	// 仅返回健康实例
	healthyInstances := make([]*ServiceInstance, 0)
	for _, instance := range l.instances {
		if instance.Alive {
			healthyInstances = append(healthyInstances, instance)
		}
	}
	return healthyInstances
}

// ReleaseConnection 释放连接计数，在请求完成后调用
func (l *LeastConnectionsBalancer) ReleaseConnection(serviceName, instanceURL string) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// 按 URL 定位实例并归还连接计数
	for _, instance := range l.instances {
		if instance.URL == instanceURL {
			instance.Connections--
			if instance.Connections < 0 {
				instance.Connections = 0
			}
			return nil
		}
	}
	return fmt.Errorf("instance %s not found in service %s", instanceURL, serviceName)
}

func (l *LeastConnectionsBalancer) SetInstanceAlive(serviceName, instanceURL string, alive bool) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	for _, instance := range l.instances {
		if instance != nil && instance.URL == instanceURL {
			instance.Alive = alive
			// 实例恢复健康后，历史连接计数重置为 0
			if alive {
				instance.Connections = 0
			}
			return nil
		}
	}
	return fmt.Errorf("instance %s not found in service %s", instanceURL, serviceName)
}
