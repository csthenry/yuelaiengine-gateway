/*
 * @Author: Henry csthenry@foxmail.com
 * @Date: 2026-03-22 15:14:03
 * @LastEditors: Henry csthenry@foxmail.com
 * @LastEditTime: 2026-03-27 18:28:24
 * @FilePath: /yuelaiengine-gateway/cmd/service-a/main.go
 * @Description:
 *
 * Copyright (c) 2026 by Henry email: csthenry@foxmail.com, All Rights Reserved.
 */
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"yuelaiengine/gateway/pkg/logger"
)

var log logger.Logger

func mainHandler(w http.ResponseWriter, r *http.Request) {
	port := getPort()
	ctx := context.Background()
	log.Debug(ctx, "Service A received request", "port", port, "path", r.URL.Path)
	fmt.Fprintf(w, "Hello from Service A at port %s, path: %s\n", port, r.URL.Path)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	port := getPort()
	ctx := context.Background()
	log.Debug(ctx, "Service A received health check request", "port", port)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	// 确保端口格式正确
	if !strings.Contains(port, ":") {
		port = ":" + port
	}
	return port
}

func main() {
	// 初始化自定义日志器
	var err error
	log, err = logger.NewWithConfigFile("./config/logs/service-a-log.yaml")
	if err != nil {
		panic(err)
	}
	ctx := context.Background()

	port := getPort()

	mux := http.NewServeMux()
	mux.HandleFunc("/", mainHandler)
	mux.HandleFunc("/healthz", healthHandler)

	server := &http.Server{
		Addr:         port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Info(ctx, "Starting Service A", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(ctx, "Could not start Service A", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info(ctx, "Service A is shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Info(ctx, "Server shutdown error", "error", err)
	}
	log.Info(ctx, "Service A stopped")
}
