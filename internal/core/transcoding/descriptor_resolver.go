/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-27 15:37:12
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-27 15:59:12
 * @FilePath: /yuelaiengine-gateway/internal/core/transcoding/descriptor_resolver.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package transcoding

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

type descriptorCacheEntry struct {
	files   *protoregistry.Files		// 解析后的 Protobuf 注册表，包含了该文件定义的所有 Service、Message 和 Enum
	size    int64						// 记录文件的原始大小和最后修改时间，如果磁盘文件没变，就直接用缓存；如果变了，就触发重新加载
	modTime time.Time					// 记录文件的原始大小和最后修改时间，如果磁盘文件没变，就直接用缓存；如果变了，就触发重新加载
}

// MethodDescriptors 保存一个 gRPC 方法的输入输出消息描述
type MethodDescriptors struct {
	FullMethod string							// 方法的全限定名（如 user.UserService/GetUser）
	Input      protoreflect.MessageDescriptor	// 输入消息描述符，根据它知道如何把收到的 JSON 拆解成 Protobuf 字段
	Output     protoreflect.MessageDescriptor	// 输出消息描述符，根据它知道如何把返回的二进制 Protobuf 包装成 JSON 键值对
}

// DescriptorResolver 负责按 descriptor set 动态解析方法描述
type DescriptorResolver struct {
	mu    sync.RWMutex
	cache map[string]descriptorCacheEntry		// Key 通常是文件路径 descriptorPath
}

func NewDescriptorResolver() *DescriptorResolver {
	return &DescriptorResolver{
		cache: make(map[string]descriptorCacheEntry),
	}
}

// ResolveGRPCMethod 从 descriptor set 中解析 gRPC 方法及输出输出消息
func (r *DescriptorResolver) ResolveGRPCMethod(descriptorPath, fullMethod string) (*MethodDescriptors, error) {
	serviceName, methodName, err := parseGRPCMethod(fullMethod)
	if err != nil {
		return nil, err
	}

	files, err := r.loadFiles(descriptorPath)
	if err != nil {
		return nil, err
	}

	serviceDesc, err := files.FindDescriptorByName(serviceName)
	if err != nil {
		return nil, fmt.Errorf("descriptor 中未找到服务 %q: %w", serviceName, err)
	}
	svc, ok := serviceDesc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%q 不是 ServiceDescriptor", serviceName)
	}
	methodDesc := svc.Methods().ByName(methodName)
	if methodDesc == nil {
		return nil, fmt.Errorf("服务 %q 中未找到方法 %q", serviceName, methodName)
	}
	
	return &MethodDescriptors{
		FullMethod: "/" + string(serviceName) + "/" + string(methodName),
		Input:      methodDesc.Input(),
		Output:     methodDesc.Output(),
	}, nil
}

func parseGRPCMethod(fullMethod string) (protoreflect.FullName, protoreflect.Name, error) {
	normalized := strings.TrimSpace(fullMethod)
	normalized = strings.TrimPrefix(normalized, "/")
	parts := strings.Split(normalized, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("grpc_method 格式非法: %q，期望 /package.Service/Method", fullMethod)
	}

	serviceName := strings.TrimSpace(parts[0])
	methodName := strings.TrimSpace(parts[1])
	if serviceName == "" || methodName == "" {
		return "", "", fmt.Errorf("grpc_method 格式非法: %q，service 或 method 为空", fullMethod)
	}

	serviceFullName := protoreflect.FullName(serviceName)
	if !serviceFullName.IsValid() {
		return "", "", fmt.Errorf("grpc_method 中 service 名非法: %q", serviceName)
	}

	methodShortName := protoreflect.Name(methodName)
	if !methodShortName.IsValid() {
		return "", "", fmt.Errorf("grpc_method 中 method 名非法: %q", methodName)
	}

	return serviceFullName, methodShortName, nil
}

func (r *DescriptorResolver) loadFiles(descriptorPath string) (*protoregistry.Files, error) {
	path := strings.TrimSpace(descriptorPath)
	if path == "" {
		return nil, fmt.Errorf("proto_descriptor_path 不能为空")
	}

	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("读取 proto_descriptor_path=%q 失败: %w", path, err)
	}

	r.mu.RLock()
	// 首先看缓存，如果缓存信息和文件信息对应，那么直接读缓存
	entry, ok := r.cache[path]
	if ok && entry.size == fi.Size() && entry.modTime.Equal(fi.ModTime()) {
		r.mu.RUnlock()
		return entry.files, nil
	}
	r.mu.RUnlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 descriptor 文件 %q 失败: %w", path, err)
	}

	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(data, &set); err != nil {
		return nil, fmt.Errorf("解析 descriptor 文件 %q 失败: %w", path, err)
	}

	files, err := protodesc.NewFiles(&set)
	if err != nil {
		return nil, fmt.Errorf("构建 descriptor 索引失败: %w", err)
	}

	r.mu.Lock()
	r.cache[path] = descriptorCacheEntry{
		files:   files,
		size:    fi.Size(),
		modTime: fi.ModTime(),
	}
	r.mu.Unlock()

	return files, nil
}
