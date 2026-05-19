package testutil

import (
	"context"

	"yuelaiengine/gateway/pkg/logger"
)

type NoopLogger struct{}

var _ logger.Logger = (*NoopLogger)(nil)

func (n *NoopLogger) Debug(ctx context.Context, msg string, fields ...interface{})  {}
func (n *NoopLogger) Info(ctx context.Context, msg string, fields ...interface{})   {}
func (n *NoopLogger) Warn(ctx context.Context, msg string, fields ...interface{})   {}
func (n *NoopLogger) Error(ctx context.Context, msg string, fields ...interface{})  {}
func (n *NoopLogger) DPanic(ctx context.Context, msg string, fields ...interface{}) {}
func (n *NoopLogger) Panic(ctx context.Context, msg string, fields ...interface{})  {}
func (n *NoopLogger) Fatal(ctx context.Context, msg string, fields ...interface{})  {}
func (n *NoopLogger) With(fields ...interface{}) logger.Logger { return n }
