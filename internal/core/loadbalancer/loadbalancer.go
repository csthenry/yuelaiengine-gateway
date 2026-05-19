/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-25 16:31:20
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-25 20:01:58
 * @FilePath: /yuelaiengine-gateway/internal/core/loadbalancer/loadbalancer.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package loadbalancer

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// ServiceInstance 服务实例
type ServiceInstance struct {
	URL         string
	Alive       bool
	Weight      int // 用于 weighted round robin (WRR) 算法
	Connections int // 用于 least connections (LC) 算法
}

// LoadBalancer 负载均衡器接口
type LoadBalancer interface {
	RegisterInstance(serviceName string, instance *ServiceInstance)
	GetNextInstance(serviceName string) (*ServiceInstance, error)
	GetAllInstances(serviceName string) []*ServiceInstance
	SetInstanceAlive(serviceName, instanceURL string, alive bool) error
	ReleaseConnection(serviceName, instanceURL string) error
}

// HashLoadBalancer 接口，提供基于 key 的实例选择能力，如一致性哈希
type HashLoadBalancer interface {
	GetInstanceByKey(serviceName, key string) (*ServiceInstance, error)
}

// 验证接口实现
var _ LoadBalancer = (*RoundRobinBalancer)(nil)
var _ LoadBalancer = (*WeightedRoundRobinBalancer)(nil)
var _ LoadBalancer = (*ConsistentHashBalancer)(nil)
var _ LoadBalancer = (*LeastConnectionsBalancer)(nil)

// LoadBalancerFactory 负载均衡器工厂
type LoadBalancerFactory struct {
	mutex sync.Mutex
	snap  atomic.Value // map[string]LoadBalancer
}

func NewLoadBalancerFactory() *LoadBalancerFactory {
	f := &LoadBalancerFactory{}
	f.snap.Store(make(map[string]LoadBalancer))
	return f
}

// GetOrCreateLoadBalancer 获取或创建负载均衡器
func (f *LoadBalancerFactory) GetOrCreateLoadBalancer(serviceName string, algorithm string) LoadBalancer {
	current := f.snap.Load().(map[string]LoadBalancer)
	lb, exists := current[serviceName]
	if exists {
		return lb
	}

	f.mutex.Lock()
	defer f.mutex.Unlock()

	latest := f.snap.Load().(map[string]LoadBalancer)
	if lb, exists := latest[serviceName]; exists {
		return lb
	}

	lb = f.newBalancer(serviceName, algorithm)
	next := make(map[string]LoadBalancer, len(latest)+1)
	for k, v := range latest {
		next[k] = v
	}
	next[serviceName] = lb
	f.snap.Store(next)
	return lb
}

func (f *LoadBalancerFactory) newBalancer(serviceName, algorithm string) LoadBalancer {
	switch algorithm {
	case "round_robin":
		return NewRoundRobinBalancer(serviceName)
	case "weighted_round_robin":
		return NewWeightedRoundRobinBalancer(serviceName)
	case "least_connections":
		return NewLeastConnectionsBalancer(serviceName)
	case "consistent_hash":
		return NewConsistentHashBalancer(serviceName)
	default:
		return NewRoundRobinBalancer(serviceName)
	}
}

// ReplaceServiceInstances 用最新配置替换服务的负载均衡器并注册实例
func (f *LoadBalancerFactory) ReplaceServiceInstances(serviceName, algorithm string, instances []*ServiceInstance) {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	current := f.snap.Load().(map[string]LoadBalancer)
	lb := f.newBalancer(serviceName, algorithm)
	for _, inst := range instances {
		lb.RegisterInstance(serviceName, inst)
	}

	next := make(map[string]LoadBalancer, len(current)+1)
	for k, v := range current {
		next[k] = v
	}
	next[serviceName] = lb
	f.snap.Store(next)
}

// RemoveService 删除服务对应负载均衡器
func (f *LoadBalancerFactory) RemoveService(serviceName string) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	current := f.snap.Load().(map[string]LoadBalancer)
	if _, ok := current[serviceName]; !ok {
		return
	}
	next := make(map[string]LoadBalancer, len(current)-1)
	for k, v := range current {
		if k == serviceName {
			continue
		}
		next[k] = v
	}
	f.snap.Store(next)
}

// ListServices 返回当前已注册负载均衡器的服务名
func (f *LoadBalancerFactory) ListServices() []string {
	current := f.snap.Load().(map[string]LoadBalancer)
	out := make([]string, 0, len(current))
	for serviceName := range current {
		out = append(out, serviceName)
	}
	return out
}

// GetInstanceByKey 根据 key 选择实例（仅当该服务使用的一致性哈希策略时可用）
func (f *LoadBalancerFactory) GetInstanceByKey(serviceName, key string) (*ServiceInstance, error) {
	current := f.snap.Load().(map[string]LoadBalancer)
	lb, ok := current[serviceName]
	if !ok {
		return nil, fmt.Errorf("service %s not found", serviceName)
	}

	hashLB, ok := lb.(HashLoadBalancer)
	if !ok {
		return nil, fmt.Errorf("service %s does not support hash selection", serviceName)
	}
	return hashLB.GetInstanceByKey(serviceName, key)
}

// UpdateInstanceAlive 同步实例健康状态到对应服务的负载均衡器
func (f *LoadBalancerFactory) UpdateInstanceAlive(serviceName, instanceURL string, alive bool) error {
	current := f.snap.Load().(map[string]LoadBalancer)
	lb, ok := current[serviceName]
	if !ok {
		return fmt.Errorf("service %s not found", serviceName)
	}

	return lb.SetInstanceAlive(serviceName, instanceURL, alive)
}

// ReleaseConnection 归还实例连接计数（LC 算法使用；其余算法 no-op）
func (f *LoadBalancerFactory) ReleaseConnection(serviceName, instanceURL string) error {
	current := f.snap.Load().(map[string]LoadBalancer)
	lb, ok := current[serviceName]
	if !ok {
		return fmt.Errorf("service %s not found", serviceName)
	}
	return lb.ReleaseConnection(serviceName, instanceURL)
}
