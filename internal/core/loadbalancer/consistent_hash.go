/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-25 17:50:52
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-25 19:40:21
 * @FilePath: /yuelaiengine-gateway/internal/core/loadbalancer/consistent_hash.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package loadbalancer

import (
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
)

const defaultVirtualNodes = 100

type hashRingEntry struct {
	hash      uint32
	instances *ServiceInstance
}

// ConsistentHashBalancer 提供 key 路由能力
type ConsistentHashBalancer struct {
	serviceName string
	instances   []*ServiceInstance
	mutex       sync.RWMutex
	ring        atomic.Value // []hashRingEntry
	counter     atomic.Uint64
}

func NewConsistentHashBalancer(serviceName string) *ConsistentHashBalancer {
	c := &ConsistentHashBalancer{
		serviceName: serviceName,
	}
	c.ring.Store(make([]hashRingEntry, 0))
	return c
}

func (c *ConsistentHashBalancer) RegisterInstance(serviceName string, instance *ServiceInstance) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.instances = append(c.instances, instance)
	c.rebuildRingLocked()
}

func (c *ConsistentHashBalancer) GetNextInstance(serviceName string) (*ServiceInstance, error) {
	seq := c.counter.Add(1)
	return c.GetInstanceByKey(serviceName, strconv.FormatUint(seq, 10))
}

func (c *ConsistentHashBalancer) GetInstanceByKey(serviceName, key string) (*ServiceInstance, error) {
	ring := c.ring.Load().([]hashRingEntry)
	instance, ok := pickFromRing(ring, key)
	if !ok || instance == nil {
		return nil, errors.New("no healthy instances available")
	}
	return instance, nil
}

func pickFromRing(ring []hashRingEntry, key string) (*ServiceInstance, bool) {
	if len(ring) == 0 {
		return nil, false
	}

	// 在环上找到第一个 hash >= target 的节点
	target := hashString(key)
	// sort.Search 是标准库中提供的一个的 Binary Search 函数
	// 在已排序的哈希环上，以 O(log N) 的效率找到第一个顺时针方向最近的节点
	idx := sort.Search(len(ring), func(i int) bool {
		return ring[i].hash >= target
	})
	if idx >= len(ring) {
		idx = 0
	}
	return ring[idx].instances, true
}

func (c *ConsistentHashBalancer) GetAllInstances(serviceName string) []*ServiceInstance {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	// 过滤出健康实例
	healthyInstances := make([]*ServiceInstance, 0)
	for _, instance := range c.instances {
		if instance.Alive {
			healthyInstances = append(healthyInstances, instance)
		}
	}
	return healthyInstances
}

func (c *ConsistentHashBalancer) rebuildRingLocked() {
	ring := make([]hashRingEntry, 0, len(c.instances)*defaultVirtualNodes)

	for _, instance := range c.instances {
		if instance == nil || !instance.Alive {
			continue
		}
		// 每个实例扩展为多个虚拟节点，降低哈希倾斜
		for i := 0; i < defaultVirtualNodes; i++ {
			virtualNodeKey := instance.URL + "#" + strconv.Itoa(i)
			ring = append(ring, hashRingEntry{
				hash:      hashString(virtualNodeKey),
				instances: instance,
			})
		}
	}

	// 按照哈希值从小到大进行升序排列，后续方便二分查找
	sort.Slice(ring, func(i, j int) bool {
		return ring[i].hash < ring[j].hash
	})

	c.ring.Store(ring)
}

// hashString 非加密型哈希函数，将任意长度的字符串压缩成一个 uint32
func hashString(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

func (c *ConsistentHashBalancer) SetInstanceAlive(serviceName, instanceURL string, alive bool) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	updated := false
	found := false
	for _, instance := range c.instances {
		if instance != nil && instance.URL == instanceURL {
			found = true
			if instance.Alive != alive {
				instance.Alive = alive
				updated = true
			}
			break
		}
	}

	// 一致性哈希环仅包含健康实例，状态变化后需要重建
	if updated {
		c.rebuildRingLocked()
	}
	if !found {
		return fmt.Errorf("instance %s not found in service %s", instanceURL, serviceName)
	}
	return nil
}

// ReleaseConnection 对一致性哈希算法是 no-op，保留统一接口。
func (c *ConsistentHashBalancer) ReleaseConnection(serviceName, instanceURL string) error {
	return nil
}
