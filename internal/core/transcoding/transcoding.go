/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-26 23:30:23
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-27 16:01:34
 * @FilePath: /yuelaiengine-gateway/internal/core/transcoding/transcoding.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package transcoding

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Options 定义路由级转码参数。
type Options struct {
	DescriptorPath string			// .pb 或 .bin 格式的 Proto 描述文件路径
	GRPCMethod     string			// 指定当前路由对应的 gRPC 方法全名
	EmitUnpopulated bool			// 如果设为 true，即使 Protobuf 字段是默认值，也会在 JSON 中显示出来
	UseProtoNames   bool			// 命名风格控制，如果设为 true，JSON 的 Key 将使用 .proto 文件中定义的原始名称
	DiscardUnknown  bool			// 如果客户端传来的 JSON 包含一些 Protobuf 定义中没有的字段，设为 true 会直接忽略它们
}

// RouteTranscoder 基于 proto 描述做 JSON<->Protobuf 的双向转码
type RouteTranscoder struct {
	method         *MethodDescriptors
	marshalOpts    protojson.MarshalOptions			// 控制输出的风格
	unmarshalOpts  protojson.UnmarshalOptions		// 控制输入的容错
	descriptorPath string
}

func NewRouteTranscoder(resolver *DescriptorResolver, opts Options) (*RouteTranscoder, error) {
	if resolver == nil {
		return nil, fmt.Errorf("descriptor resolver 不能为空")
	}

	method, err := resolver.ResolveGRPCMethod(opts.DescriptorPath, opts.GRPCMethod)
	if err != nil {
		return nil, err
	}

	return &RouteTranscoder{
		method: method,
		marshalOpts: protojson.MarshalOptions{
			UseProtoNames:   opts.UseProtoNames,
			EmitUnpopulated: opts.EmitUnpopulated,
		},
		unmarshalOpts: protojson.UnmarshalOptions{
			DiscardUnknown: opts.DiscardUnknown,
		},
		descriptorPath: opts.DescriptorPath,
	}, nil
}

func (t *RouteTranscoder) JSONToProtobufRequest(jsonBody []byte) ([]byte, error) {
	return t.jsonToProtoBytes(t.method.Input, jsonBody)
}

func (t *RouteTranscoder) JSONToProtobufResponse(jsonBody []byte) ([]byte, error) {
	return t.jsonToProtoBytes(t.method.Output, jsonBody)
}

func (t *RouteTranscoder) ProtobufRequestToJSON(protoBody []byte) ([]byte, error) {
	return t.protoBytesToJSON(t.method.Input, protoBody)
}

func (t *RouteTranscoder) ProtobufResponseToJSON(protoBody []byte) ([]byte, error) {
	return t.protoBytesToJSON(t.method.Output, protoBody)
}

func (t *RouteTranscoder) JSONToGRPCRequest(jsonBody []byte) ([]byte, error) {
	protoBody, err := t.JSONToProtobufRequest(jsonBody)
	if err != nil {
		return nil, err
	}
	return WrapGRPCFrame(protoBody), nil
}

func (t *RouteTranscoder) JSONToGRPCResponse(jsonBody []byte) ([]byte, error) {
	protoBody, err := t.JSONToProtobufResponse(jsonBody)
	if err != nil {
		return nil, err
	}
	return WrapGRPCFrame(protoBody), nil
}

func (t *RouteTranscoder) GRPCRequestToJSON(grpcBody []byte) ([]byte, error) {
	protoBody, err := UnwrapGRPCFrame(grpcBody)
	if err != nil {
		return nil, err
	}
	return t.ProtobufRequestToJSON(protoBody)
}

func (t *RouteTranscoder) GRPCResponseToJSON(grpcBody []byte) ([]byte, error) {
	protoBody, err := UnwrapGRPCFrame(grpcBody)
	if err != nil {
		return nil, err
	}
	return t.ProtobufResponseToJSON(protoBody)
}

// WrapGRPCFrame 将 protobuf payload 打包成 unary gRPC frame。
func WrapGRPCFrame(payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = 0 // 压缩标记：0 = 未压缩
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}

// UnwrapGRPCFrame 从 unary gRPC frame 中提取 protobuf payload。
func UnwrapGRPCFrame(frame []byte) ([]byte, error) {
	if len(frame) < 5 {
		return nil, fmt.Errorf("grpc frame 太短: %d", len(frame))
	}
	if frame[0] != 0 {
		return nil, fmt.Errorf("暂不支持压缩 gRPC frame，flag=%d", frame[0])
	}

	length := int(binary.BigEndian.Uint32(frame[1:5]))
	if length < 0 {
		return nil, fmt.Errorf("grpc frame 长度非法: %d", length)
	}
	if len(frame) != 5+length {
		return nil, fmt.Errorf("grpc frame 长度不匹配: 标头=%d 实际payload=%d", length, len(frame)-5)
	}

	payload := make([]byte, length)
	copy(payload, frame[5:])
	return payload, nil
}

func (t *RouteTranscoder) jsonToProtoBytes(desc protoreflect.MessageDescriptor, jsonBody []byte) ([]byte, error) {
	msg := dynamicpb.NewMessage(desc)

	normalized := bytes.TrimSpace(jsonBody)
	if len(normalized) == 0 {
		normalized = []byte("{}")
	}
	if err := t.unmarshalOpts.Unmarshal(normalized, msg); err != nil {
		return nil, fmt.Errorf("JSON->Protobuf 失败(method=%s): %w", t.method.FullMethod, err)
	}

	out, err := proto.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("Protobuf 编码失败(method=%s): %w", t.method.FullMethod, err)
	}
	return out, nil
}

func (t *RouteTranscoder) protoBytesToJSON(desc protoreflect.MessageDescriptor, protoBody []byte) ([]byte, error) {
	msg := dynamicpb.NewMessage(desc)
	if len(protoBody) > 0 {
		if err := proto.Unmarshal(protoBody, msg); err != nil {
			return nil, fmt.Errorf("Protobuf->JSON 失败(method=%s): %w", t.method.FullMethod, err)
		}
	}

	out, err := t.marshalOpts.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("JSON 编码失败(method=%s): %w", t.method.FullMethod, err)
	}
	return out, nil
}
