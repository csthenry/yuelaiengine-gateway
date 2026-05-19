/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-22 15:13:54
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-24 21:38:44
 * @FilePath: /yuelaiengine-gateway/cmd/api-gateway/main.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package main

import (
	"context"
	"time"

	"yuelaiengine/gateway/internal/api"
	"yuelaiengine/gateway/internal/config"
	"yuelaiengine/gateway/internal/core"
	"yuelaiengine/gateway/pkg/logger"
)

func main() {
	// 初始化日志
	logger, err := logger.NewWithConfigFile("./config/logs/api-gateway-log.yml")
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	// 加载配置
	logger.Info(ctx, "加载配置中...")
	cfg, err := config.Load("./config/config.yml")
	if err != nil {
		logger.Fatal(ctx, "致命错误: 加载配置失败", "error", err)
	}
	logger.Info(ctx, "配置加载成功。")

	// 创建网关实例
	logger.Info(ctx, "初始化网关层...")
	gw, err := core.NewGateway(cfg, logger)
	if err != nil {
		logger.Fatal(ctx, "创建网关失败", "error", err)
	}
	gw.SetConfigPath("./config/config.yml")
	logger.Info(ctx, "网关层初始化成功。")

	// 配置热更新
	if cfg.HotReload.Enabled {
		interval := cfg.HotReload.Interval
		if interval <= 0 {
			interval = 3 * time.Second
		}
		gw.StartAutoReload("./config/config.yml", interval)
		logger.Info(ctx, "配置热更新已启用", "interval", interval.String())
	}

	// 使用 Hertz(Netpoll) 承接网络入口，内部继续复用现有网关处理链路。
	hz := api.NewHertzServer(gw, logger, cfg.Server.Port)
	logger.Info(ctx, "Hertz 服务器创建完成", "port", cfg.Server.Port)
	hz.Spin()
	gw.Shutdown()
}
