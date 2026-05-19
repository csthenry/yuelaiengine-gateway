/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-27 16:17:56
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-27 17:08:07
 * @FilePath: /yuelaiengine-gateway/internal/core/proxy/transcoding.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */

// 协议转换子模块
// - prepareTranscoder：按 route 配置构建转码器
// - applyRequestTranscoding：请求侧转码
// - apply*ResponseTranscoding：响应侧转码
//
// - descriptor 解析和 message 编解码在 internal/core/transcoding/*
package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/core/transcoding"
)

func (p *Proxy) prepareTranscoder(route *config.RouteConfig) (string, *transcoding.RouteTranscoder, error) {
	mode := strings.ToLower(strings.TrimSpace(route.ProtocolConvert))
	if mode == "" || mode == "none" {
		return "", nil, nil
	}

	switch mode {
	case "http_json_to_grpc", "grpc_to_http_json":
	default:
		return "", nil, fmt.Errorf("不支持的 protocol_convert=%q", route.ProtocolConvert)
	}

	transcoder, err := transcoding.NewRouteTranscoder(p.descriptorLoader, transcoding.Options{
		DescriptorPath:  route.ProtoDescriptor,
		GRPCMethod:      route.GRPCMethod,
		EmitUnpopulated: route.EmitUnpopulated,
		UseProtoNames:   route.UseProtoNames,
		DiscardUnknown:  route.DiscardUnknown,
	})
	if err != nil {
		return "", nil, err
	}
	return mode, transcoder, nil
}

func applyRequestTranscoding(r *http.Request, mode string, transcoder *transcoding.RouteTranscoder) error {
	if transcoder == nil {
		return nil
	}

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("读取请求体失败: %w", err)
	}
	_ = r.Body.Close()

	switch mode {
	case "http_json_to_grpc":
		grpcBody, err := transcoder.JSONToGRPCRequest(rawBody)
		if err != nil {
			return err
		}
		rewriteRequestBody(r, grpcBody)
		r.Header.Set("Content-Type", "application/grpc")
	case "grpc_to_http_json":
		jsonBody, err := transcoder.GRPCRequestToJSON(rawBody)
		if err != nil {
			return err
		}
		rewriteRequestBody(r, jsonBody)
		r.Header.Set("Content-Type", "application/json")
	}
	return nil
}

func rewriteRequestBody(r *http.Request, body []byte) {
	// no-op 代表什么也不做，只是将其包装为一个 io.ReadCloser
	// bytes.Reader 满足不了 ReadCloser 接口，不能直接赋值给 resp.Body
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}

	if len(body) == 0 {
		r.Header.Del("Content-Length")
		return
	}
	r.Header.Set("Content-Length", strconv.Itoa(len(body)))
}

func applyGRPCToJSONResponseTranscoding(resp *http.Response, transcoder *transcoding.RouteTranscoder) error {
	rawBody, err := readAndReplaceResponseBody(resp)
	if err != nil {
		return &proxyHTTPError{
			StatusCode: http.StatusBadGateway,
			Message:    "读取上游响应失败",
			Err:        err,
		}
	}

	grpcStatus, grpcMessage := grpcStatusAndMessage(resp)
	if grpcStatus != "" && grpcStatus != "0" {
		code, parseErr := strconv.Atoi(grpcStatus)
		if parseErr != nil {
			code = 13 // INTERNAL
		}

		payload, err := json.Marshal(map[string]interface{}{
			"error": map[string]string{
				"grpc_status":  grpcStatus,
				"grpc_message": grpcMessage,
			},
		})
		if err != nil {
			return &proxyHTTPError{
				StatusCode: http.StatusBadGateway,
				Message:    "编码 gRPC 错误响应失败",
				Err:        err,
			}
		}

		rewriteResponse(resp, grpcCodeToHTTPStatus(code), "application/json", payload)
		cleanupGRPCResponseHeaders(resp)
		return nil
	}

	jsonBody, err := transcoder.GRPCResponseToJSON(rawBody)
	if err != nil {
		return &proxyHTTPError{
			StatusCode: http.StatusBadGateway,
			Message:    "gRPC->JSON 转码失败",
			Err:        err,
		}
	}

	rewriteResponse(resp, http.StatusOK, "application/json", jsonBody)
	cleanupGRPCResponseHeaders(resp)
	return nil
}

func applyJSONToGRPCResponseTranscoding(resp *http.Response, transcoder *transcoding.RouteTranscoder) error {
	rawBody, err := readAndReplaceResponseBody(resp)
	if err != nil {
		return &proxyHTTPError{
			StatusCode: http.StatusBadGateway,
			Message:    "读取上游响应失败",
			Err:        err,
		}
	}

	if resp.StatusCode >= 400 {
		originalStatus := resp.StatusCode
		message := strings.TrimSpace(string(rawBody))
		if message == "" {
			message = http.StatusText(originalStatus)
		}
		rewriteResponse(resp, http.StatusOK, "application/grpc", []byte{})
		resp.Header.Set("Grpc-Status", strconv.Itoa(httpStatusToGRPCCode(originalStatus)))
		resp.Header.Set("Grpc-Message", url.QueryEscape(message))
		return nil
	}

	grpcBody, err := transcoder.JSONToGRPCResponse(rawBody)
	if err != nil {
		return &proxyHTTPError{
			StatusCode: http.StatusBadGateway,
			Message:    "JSON->gRPC 转码失败",
			Err:        err,
		}
	}

	rewriteResponse(resp, http.StatusOK, "application/grpc", grpcBody)
	resp.Header.Set("Grpc-Status", "0")
	return nil
}

func readAndReplaceResponseBody(resp *http.Response) ([]byte, error) {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func rewriteResponse(resp *http.Response, statusCode int, contentType string, body []byte) {
	resp.StatusCode = statusCode
	resp.Status = fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode))
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Type", contentType)
	if len(body) == 0 {
		resp.Header.Del("Content-Length")
	} else {
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	}
}

func cleanupGRPCResponseHeaders(resp *http.Response) {
	resp.Header.Del("Grpc-Status")
	resp.Header.Del("Grpc-Message")
	resp.Header.Del("Grpc-Encoding")
	resp.Header.Del("Grpc-Accept-Encoding")
	resp.Header.Del("Trailer")
	resp.Trailer = nil
}

func grpcStatusAndMessage(resp *http.Response) (string, string) {
	status := strings.TrimSpace(resp.Header.Get("Grpc-Status"))
	if status == "" {
		status = strings.TrimSpace(resp.Trailer.Get("Grpc-Status"))
	}

	message := strings.TrimSpace(resp.Header.Get("Grpc-Message"))
	if message == "" {
		message = strings.TrimSpace(resp.Trailer.Get("Grpc-Message"))
	}

	if decoded, err := url.QueryUnescape(message); err == nil {
		message = decoded
	}
	return status, message
}

func grpcCodeToHTTPStatus(code int) int {
	switch code {
	case 0:
		return http.StatusOK
	case 1, 2, 13, 15:
		return http.StatusInternalServerError
	case 3:
		return http.StatusBadRequest
	case 4:
		return http.StatusGatewayTimeout
	case 5:
		return http.StatusNotFound
	case 6:
		return http.StatusConflict
	case 7:
		return http.StatusForbidden
	case 8:
		return http.StatusTooManyRequests
	case 9:
		return http.StatusBadRequest
	case 10:
		return http.StatusConflict
	case 11:
		return http.StatusBadRequest
	case 12:
		return http.StatusNotImplemented
	case 14:
		return http.StatusServiceUnavailable
	case 16:
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}

func httpStatusToGRPCCode(status int) int {
	switch status {
	case http.StatusBadRequest:
		return 3 // INVALID_ARGUMENT
	case http.StatusUnauthorized:
		return 16 // UNAUTHENTICATED
	case http.StatusForbidden:
		return 7 // PERMISSION_DENIED
	case http.StatusNotFound:
		return 5 // NOT_FOUND
	case http.StatusConflict:
		return 10 // ABORTED
	case http.StatusTooManyRequests:
		return 8 // RESOURCE_EXHAUSTED
	case http.StatusNotImplemented:
		return 12 // UNIMPLEMENTED
	case http.StatusBadGateway, http.StatusServiceUnavailable:
		return 14 // UNAVAILABLE
	case http.StatusGatewayTimeout:
		return 4 // DEADLINE_EXCEEDED
	default:
		if status >= 500 {
			return 13 // INTERNAL
		}
		return 2 // UNKNOWN
	}
}
