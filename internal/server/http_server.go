package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/roowe/tushareproxy/internal/api"
	"github.com/roowe/tushareproxy/internal/config"
	"github.com/roowe/tushareproxy/pkg/logger"

	"go.uber.org/zap"
)

// HTTPServer HTTP服务器结构体
type HTTPServer struct {
	server *http.Server
	config *config.ServerConfig
}

// NewHTTPServer 创建新的HTTP服务器实例
func NewHTTPServer(cfg *config.ServerConfig) *HTTPServer {
	return &HTTPServer{
		config: cfg,
	}
}

// Start 启动HTTP服务器
func (s *HTTPServer) Start() error {
	// 创建多路复用器
	mux := http.NewServeMux()

	// 注册路由
	s.registerRoutes(mux)

	// 创建HTTP服务器
	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Host, s.config.Port),
		Handler:      mux,
		ReadTimeout:  time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.WriteTimeout) * time.Second,
	}

	logger.Info("HTTP服务器启动",
		zap.String("address", s.server.Addr),
		zap.Int("read_timeout", s.config.ReadTimeout),
		zap.Int("write_timeout", s.config.WriteTimeout))

	return s.server.ListenAndServe()
}

// Stop 停止HTTP服务器
func (s *HTTPServer) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	logger.Info("正在停止HTTP服务器")
	return s.server.Shutdown(ctx)
}

// registerRoutes 注册路由
func (s *HTTPServer) registerRoutes(mux *http.ServeMux) {
	// 注册/dataapi路由
	mux.HandleFunc("/dataapi", api.DataAPIHandler)
}
