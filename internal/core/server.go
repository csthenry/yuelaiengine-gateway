/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-22 15:16:10
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-24 21:40:56
 * @FilePath: /yuelaiengine-gateway/internal/core/server.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package core

import (
	"os"
	"context"
	"net/http"
	"os/signal"
	"syscall"
	"time"
	"yuelaiengine/gateway/pkg/logger"
)

type Server struct {
	httpServer *http.Server
	logger     logger.Logger
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info(ctx, "服务器关闭中...")
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error(ctx, "服务器异常关闭", "error", err)
		return err
	}
	s.logger.Info(ctx, "服务器已关闭")
	return nil
}

func (s *Server) Start() error	{
	s.logger.Info(context.Background(), "服务器运行中", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func NewServer(addr string, handler http.Handler, logger logger.Logger) *Server {
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
		ReadTimeout: 5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout: 120 * time.Second,
	}
	return &Server{
		httpServer: srv,
		logger: logger,
	}
}

func (s *Server) WaitShutdown() error {
	quit := make(chan os.Signal, 1)
	// 监听 Ctrl + C / kill
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 阻塞，直到接收信号
	<-quit

	// 执行剩余请求
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(ctx)
}