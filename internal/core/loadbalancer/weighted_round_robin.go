/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-25 16:34:22
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-25 17:55:13
 * @FilePath: /yuelaiengine-gateway/internal/core/loadbalancer/weighted_round_robin.go
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

type WeightedRoundRobinBalancer struct {
	serviceName string
	instances   []*ServiceInstance
	mutex       sync.RWMutex
	current     atomic.Uint64 // 全局请求序号
}

func NewWeightedRoundRobinBalancer(serviceName string) *WeightedRoundRobinBalancer {
	return &WeightedRoundRobinBalancer{
		serviceName: serviceName,
	}
}

func (w *WeightedRoundRobinBalancer) RegisterInstance(serviceName string, instance *ServiceInstance) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.instances = append(w.instances, instance)
}

func (w *WeightedRoundRobinBalancer) GetNextInstance(serviceName string) (*ServiceInstance, error) {
	w.mutex.RLock()

	if len(w.instances) == 0 {
		w.mutex.RUnlock()
		return nil, errors.New("no instances available")
	}

	// 过滤出健康的实例
	healthyInstances := make([]*ServiceInstance, 0)
	weightedInstances := make([]*ServiceInstance, 0)
	totalWeight := 0
	for _, instance := range w.instances {
		if instance.Alive {
			healthyInstances = append(healthyInstances, instance)
			if instance.Weight > 0 {
				weightedInstances = append(weightedInstances, instance)
				totalWeight += instance.Weight
			}
		}
	}

	if len(healthyInstances) == 0 {
		w.mutex.RUnlock()
		return nil, errors.New("no healthy instances available")
	}
	w.mutex.RUnlock()

	// 原子方式获取当前序号，避免并发竞争
	seq := w.current.Add(1) - 1

	// 如果权重缺省或配置为非正数，回退为普通轮询(RR)
	if totalWeight == 0 {
		// seq % len(healthyInstances) 避免越界
		instance := healthyInstances[int(seq%uint64(len(healthyInstances)))]
		return instance, nil
	}

	// WRR 将 current 投影至总权重区间，用累计权重定位实例
	// [0, totalWeight)
	target := int(seq % uint64(totalWeight))
	selectedInstance := weightedInstances[0]
	cumulativeWeight := 0

	// 累加权重，如果当前实例权重
	// 假设三个实例 [ A | B | C ) 属于 [0, totalWeight)
	// 如果 target 落在 B 区间，即满足 A.Weight + B.Weight > target
	// 此时 selectedInstance = B
	for _, instance := range weightedInstances {
		cumulativeWeight += instance.Weight
		if target < cumulativeWeight {
			selectedInstance = instance
			break
		}
	}

	return selectedInstance, nil
}

// GetAllInstances 返回所有健康实例
func (w *WeightedRoundRobinBalancer) GetAllInstances(serviceName string) []*ServiceInstance {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	// 过滤健康实例
	healthyInstances := make([]*ServiceInstance, 0)
	for _, instance := range w.instances {
		if instance.Alive {
			healthyInstances = append(healthyInstances, instance)
		}
	}
	return healthyInstances
}

func (w *WeightedRoundRobinBalancer) SetInstanceAlive(serviceName, instanceURL string, alive bool) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	for _, instance := range w.instances {
		if instance != nil && instance.URL == instanceURL {
			instance.Alive = alive
			return nil
		}
	}
	return fmt.Errorf("instance %s not found in service %s", instanceURL, serviceName)
}

// ReleaseConnection 对 WRR 算法是 no-op，保留统一接口。
func (w *WeightedRoundRobinBalancer) ReleaseConnection(serviceName, instanceURL string) error {
	return nil
}
