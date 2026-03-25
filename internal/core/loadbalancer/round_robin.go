/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-25 17:39:44
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-25 17:56:32
 * @FilePath: /yuelaiengine-gateway/internal/core/loadbalancer/round_robin.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package loadbalancer

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

type RoundRobinBalancer struct {
	serviceName string
	instances   []*ServiceInstance
	mutex       sync.RWMutex
	index       atomic.Uint64 // 全局递增游标，配合取模实现轮询
}

func NewRoundRobinBalancer(serviceName string) *RoundRobinBalancer {
	return &RoundRobinBalancer{
		serviceName: serviceName,
	}
}

func (r *RoundRobinBalancer) RegisterInstance(serviceName string, instance *ServiceInstance) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// 实例按注册顺序进入池，轮询顺序也基于该顺序
	r.instances = append(r.instances, instance)
}

func (r *RoundRobinBalancer) GetNextInstance(serviceName string) (*ServiceInstance, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	if len(r.instances) == 0 {
		return nil, errors.New("no instances available")
	}

	// 过滤健康实例
	healthyInstances := make([]*ServiceInstance, 0)
	for _, instance := range r.instances {
		if instance.Alive {
			healthyInstances = append(healthyInstances, instance)
		}
	}

	if len(healthyInstances) == 0 {
		return nil, errors.New("no healthy instances available")
	}

	// 原子方式获取当前序号，避免并发竞争
	seq := r.index.Add(1) - 1
	// 通过 seq % N 在健康实例集合中做环形游走
	instance := healthyInstances[int(seq%uint64(len(healthyInstances)))]

	return instance, nil
}

func (r *RoundRobinBalancer) GetAllInstances(serviceName string) []*ServiceInstance {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// 仅暴露健康实例
	healthyInstances := make([]*ServiceInstance, 0)
	for _, instance := range r.instances {
		if instance.Alive {
			healthyInstances = append(healthyInstances, instance)
		}
	}
	return healthyInstances
}

func (r *RoundRobinBalancer) SetInstanceAlive(serviceName, instanceURL string, alive bool) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	for _, instance := range r.instances {
		if instance != nil && instance.URL == instanceURL {
			instance.Alive = alive
			return nil
		}
	}
	return fmt.Errorf("instance %s not found in service %s", instanceURL, serviceName)
}

// ReleaseConnection 对 RR 算法是 no-op，保留统一接口。
func (r *RoundRobinBalancer) ReleaseConnection(serviceName, instanceURL string) error {
	return nil
}
