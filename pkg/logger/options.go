/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-22 15:21:31
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-22 15:33:59
 * @FilePath: /yuelaiengine-gateway/pkg/logger/options.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package logger

type Options struct {
	Level            string          `yaml:"level"`             // 日志级别
	Format           string          `yaml:"format"`            // 格式 json/console
	OutputPaths      []string        `yaml:"output_paths"`      // 输出路径列表
	ErrorPaths       []string        `yaml:"error_paths"`       // 错误日志输出路径列表
	EnableCaller     bool            `yaml:"enable_caller"`     // 是否启用调用者信息
	EnableStacktrace bool            `yaml:"enable_stacktrace"` // 是否启用堆栈跟踪
	StacktraceLevel  string          `yaml:"stacktrace_level"`  // 堆栈跟踪级别
	Rotation         RotationOptions `yaml:"rotation"`          // 日志滚动配置
}

type RotationOptions struct {
	Enabled bool   `yaml:"enabled"` // 是否启用
	Policy  string `yaml:"policy"`  // 滚动策略（time/size/size_and_time）
	// based on time
	TimeInterval string `yaml:"time_interval"` //轮转时间间隔 (minute/hour/24h)
	// based on size
	MaxSize   int  `yaml:"max_size"`   // 日志文件最大大小 MB
	MaxBackups int  `yaml:"max_backups"` // 最大备份数量
	MaxAge    int  `yaml:"max_age"`    // 最大保留天数
	Compress  bool `yaml:"compress"`   // 是否压缩
}

// 函数选项模式
type Option func(*Options)

func WithLevel(level string) Option {
	return func(o *Options) {
		o.Level = level
	}
}

func WithFormat(format string) Option {
	return func(o *Options) {
		o.Format = format
	}
}

func WithOutputPaths(outputPaths []string) Option {
	return func(o *Options) {
		o.OutputPaths = outputPaths
	}
}

func WithErrorPaths(errorPaths []string) Option {
	return func(o *Options) {
		o.ErrorPaths = errorPaths
	}
}

func WithCaller(enableCaller bool) Option {
	return func(o *Options) {
		o.EnableCaller = enableCaller
	}
}

func WithStacktrace(enableStacktrace bool) Option {
	return func(o *Options) {
		o.EnableStacktrace = enableStacktrace
	}
}

func WihtStacktraceLevel(stacktraceLevel string) Option {
	return func(o *Options) {
		o.StacktraceLevel = stacktraceLevel
	}
}

func WithRotation(rotation RotationOptions) Option {
	return func(o *Options) {
		o.Rotation = rotation
	}
}
