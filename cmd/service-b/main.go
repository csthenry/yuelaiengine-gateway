/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-22 15:14:09
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-05-19 19:10:00
 * @FilePath: /yuelaiengine-gateway/cmd/service-b/main.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"yuelaiengine/gateway/internal/core/transcoding"
	"yuelaiengine/gateway/pkg/logger"
)

const (
	grpcEchoMethod     = "/gateway.serviceb.v1.EchoService/Echo"
	defaultDescriptor  = "./config/proto/service-b-echo.pb"
	defaultServicePort = "8083"
)

var log logger.Logger

type grpcMethodSchema struct {
	input  protoreflect.MessageDescriptor
	output protoreflect.MessageDescriptor
}

func mainHandler(w http.ResponseWriter, r *http.Request) {
	port := getPort()
	ctx := context.Background()
	log.Info(ctx, "Service B received HTTP request", "port", port, "path", r.URL.Path)
	fmt.Fprintf(w, "Hello from Service B at port %s, path: %s\n", port, r.URL.Path)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	port := getPort()
	ctx := context.Background()
	log.Info(ctx, "Service B received health check request", "port", port)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

func grpcEchoHandler(schema *grpcMethodSchema) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != grpcEchoMethod {
			http.NotFound(w, r)
			return
		}
		if !strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/grpc") {
			writeGRPCError(w, 3, "content-type must be application/grpc")
			return
		}

		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			writeGRPCError(w, 13, "failed to read request body")
			return
		}
		_ = r.Body.Close()

		reqPayload, err := transcoding.UnwrapGRPCFrame(rawBody)
		if err != nil {
			writeGRPCError(w, 3, "invalid grpc frame")
			return
		}

		reqMsg := dynamicpb.NewMessage(schema.input)
		if err := proto.Unmarshal(reqPayload, reqMsg); err != nil {
			writeGRPCError(w, 3, "invalid protobuf payload")
			return
		}

		name := getStringField(reqMsg, schema.input, "name")
		userID := getStringField(reqMsg, schema.input, "user_id")
		age := getInt32Field(reqMsg, schema.input, "age")
		if strings.TrimSpace(name) == "" {
			name = "anonymous"
		}

		respMsg := dynamicpb.NewMessage(schema.output)
		setStringField(respMsg, schema.output, "message", fmt.Sprintf("Hello %s from Service B gRPC (user_id=%s, age=%d)", name, userID, age))
		setBoolField(respMsg, schema.output, "ok", true)
		setStringField(respMsg, schema.output, "server", getPort())

		respPayload, err := proto.Marshal(respMsg)
		if err != nil {
			writeGRPCError(w, 13, "failed to encode response")
			return
		}

		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status, Grpc-Message")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(transcoding.WrapGRPCFrame(respPayload))
		w.Header().Set("Grpc-Status", "0")
		w.Header().Set("Grpc-Message", "")

		log.Info(ctx, "Service B handled gRPC echo request", "name", name, "user_id", userID, "age", age)
	}
}

func writeGRPCError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/grpc")
	w.Header().Set("Trailer", "Grpc-Status, Grpc-Message")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(transcoding.WrapGRPCFrame(nil))
	w.Header().Set("Grpc-Status", fmt.Sprintf("%d", code))
	w.Header().Set("Grpc-Message", message)
}

func getStringField(msg *dynamicpb.Message, desc protoreflect.MessageDescriptor, fieldName protoreflect.Name) string {
	fd := desc.Fields().ByName(fieldName)
	if fd == nil || !msg.Has(fd) {
		return ""
	}
	return msg.Get(fd).String()
}

func getInt32Field(msg *dynamicpb.Message, desc protoreflect.MessageDescriptor, fieldName protoreflect.Name) int32 {
	fd := desc.Fields().ByName(fieldName)
	if fd == nil || !msg.Has(fd) {
		return 0
	}
	return int32(msg.Get(fd).Int())
}

func setStringField(msg *dynamicpb.Message, desc protoreflect.MessageDescriptor, fieldName protoreflect.Name, value string) {
	fd := desc.Fields().ByName(fieldName)
	if fd != nil {
		msg.Set(fd, protoreflect.ValueOfString(value))
	}
}

func setBoolField(msg *dynamicpb.Message, desc protoreflect.MessageDescriptor, fieldName protoreflect.Name, value bool) {
	fd := desc.Fields().ByName(fieldName)
	if fd != nil {
		msg.Set(fd, protoreflect.ValueOfBool(value))
	}
}

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultServicePort
	}
	if !strings.Contains(port, ":") {
		port = ":" + port
	}
	return port
}

func getDescriptorPath() string {
	if p := strings.TrimSpace(os.Getenv("SERVICE_B_DESCRIPTOR_PATH")); p != "" {
		return p
	}
	return defaultDescriptor
}

func buildEchoDescriptorSet() *descriptorpb.FileDescriptorSet {
	file := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("gateway/service_b/echo.proto"),
		Package: proto.String("gateway.serviceb.v1"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("EchoRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("name"),
						Number: proto.Int32(1),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
					{
						Name:     proto.String("user_id"),
						JsonName: proto.String("userId"),
						Number:   proto.Int32(2),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
					{
						Name:   proto.String("age"),
						Number: proto.Int32(3),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
					},
				},
			},
			{
				Name: proto.String("EchoResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("message"),
						Number: proto.Int32(1),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
					{
						Name:   proto.String("ok"),
						Number: proto.Int32(2),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum(),
					},
					{
						Name:   proto.String("server"),
						Number: proto.Int32(3),
						Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
					},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("EchoService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("Echo"),
						InputType:  proto.String(".gateway.serviceb.v1.EchoRequest"),
						OutputType: proto.String(".gateway.serviceb.v1.EchoResponse"),
					},
				},
			},
		},
	}
	return &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{file}}
}

func prepareGRPCSchema(descriptorPath string) (*grpcMethodSchema, error) {
	set := buildEchoDescriptorSet()

	raw, err := proto.Marshal(set)
	if err != nil {
		return nil, fmt.Errorf("marshal descriptor set failed: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(descriptorPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir descriptor dir failed: %w", err)
	}
	if err := os.WriteFile(descriptorPath, raw, 0o644); err != nil {
		return nil, fmt.Errorf("write descriptor file failed: %w", err)
	}

	files, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, fmt.Errorf("build descriptors failed: %w", err)
	}

	svcDescRaw, err := files.FindDescriptorByName("gateway.serviceb.v1.EchoService")
	if err != nil {
		return nil, fmt.Errorf("find service descriptor failed: %w", err)
	}
	svcDesc, ok := svcDescRaw.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("descriptor type mismatch for service")
	}

	method := svcDesc.Methods().ByName("Echo")
	if method == nil {
		return nil, fmt.Errorf("echo method not found in descriptor")
	}

	return &grpcMethodSchema{
		input:  method.Input(),
		output: method.Output(),
	}, nil
}

func grpcInfoHandler(w http.ResponseWriter, r *http.Request) {
	info := map[string]string{
		"grpc_method":       grpcEchoMethod,
		"descriptor_path":   getDescriptorPath(),
		"content_type":      "application/grpc",
		"protocol":          "h2c",
		"request_json_hint": `{"name":"alice","userId":"u-1","age":18}`,
	}
	_ = json.NewEncoder(w).Encode(info)
}

func main() {
	var err error
	log, err = logger.NewWithConfigFile("./config/logs/service-b-log.yaml")
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	schema, err := prepareGRPCSchema(getDescriptorPath())
	if err != nil {
		log.Fatal(ctx, "Prepare Service B gRPC schema failed", "error", err)
	}

	port := getPort()
	mux := http.NewServeMux()
	mux.HandleFunc("/", mainHandler)
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/_grpc/info", grpcInfoHandler)
	mux.HandleFunc(grpcEchoMethod, grpcEchoHandler(schema))

	server := &http.Server{
		Addr:         port,
		Handler:      h2c.NewHandler(mux, &http2.Server{}),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Info(ctx, "Starting Service B", "port", port, "grpc_method", grpcEchoMethod, "descriptor_path", getDescriptorPath())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(ctx, "Could not start Service B", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info(ctx, "Service B is shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Info(ctx, "Server shutdown error", "error", err)
	}
	log.Info(ctx, "Service B stopped")
}
