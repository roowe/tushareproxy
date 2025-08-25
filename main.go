package main

import (
	"context"
	"time"

	"github.com/roowe/tushareproxy/internal/api"
	"github.com/roowe/tushareproxy/internal/cache"
	"github.com/roowe/tushareproxy/internal/config"
	"github.com/roowe/tushareproxy/internal/server"

	"os"
	"os/signal"
	"syscall"

	"github.com/roowe/tushareproxy/pkg/logger"

	"go.uber.org/zap"
)

func main() {
	// 初始化日志
	err := logger.InitDefaultLogger()
	if err != nil {
		panic(err)
	}

	// 读取配置文件
	configPath := ""
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	if err := config.InitConfigFromPath(configPath); err != nil {
		logger.Fatal("读取配置文件失败", zap.Error(err))
	}
	cfg := config.GetConfig()
	logger.Debug("配置加载成功", zap.Any("config", cfg))
	err = logger.InitLogger(&cfg.Log)
	if err != nil {
		panic(err)
	}
	logger.Debug("config and logger init success")

	// 初始化缓存
	var cacheManager *cache.CacheManager
	if cfg.Cache.Enabled {
		cacheManager, err = cache.NewCacheManager(cfg.Cache.DBPath, cfg.Cache.TTLDays)
		if err != nil {
			logger.Fatal("初始化缓存失败", zap.Error(err))
		}
		// 设置全局缓存管理器
		api.SetCacheManager(cacheManager)
		// 启动垃圾回收例程
		cacheManager.StartGCRoutine()
		logger.Info("缓存系统初始化成功")
	} else {
		logger.Info("缓存功能已禁用")
	}

	// 创建HTTP服务器
	httpServer := server.NewHTTPServer(&cfg.Server)

	// 设置优雅关闭
	setupGracefulShutdown(httpServer, cacheManager)

	// 启动HTTP服务器
	logger.Info("正在启动HTTP服务器...")
	if err := httpServer.Start(); err != nil {
		logger.Fatal("HTTP服务器启动失败", zap.Error(err))
	}
}

// 设置优雅关闭
func setupGracefulShutdown(httpServer *server.HTTPServer, cacheManager *cache.CacheManager) {
	// 创建信号通道
	sigChan := make(chan os.Signal, 1)

	// 监听系统信号
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 在后台处理信号
	go func() {
		sig := <-sigChan
		logger.Info("收到关闭信号，开始优雅关闭", zap.String("signal", sig.String()))

		// 执行优雅关闭流程
		gracefulShutdown(httpServer, cacheManager)

		// 退出程序
		os.Exit(0)
	}()
}

// 优雅关闭流程
func gracefulShutdown(httpServer *server.HTTPServer, cacheManager *cache.CacheManager) {
	logger.Info("开始优雅关闭流程")

	// 创建关闭上下文，给服务器30秒时间优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 停止HTTP服务器
	if httpServer != nil {
		logger.Info("正在停止HTTP服务器")
		if err := httpServer.Stop(ctx); err != nil {
			logger.Error("停止HTTP服务器失败", zap.Error(err))
		} else {
			logger.Info("HTTP服务器已停止")
		}
	}

	// 关闭缓存
	if cacheManager != nil {
		logger.Info("正在关闭缓存系统")
		if err := cacheManager.Close(); err != nil {
			logger.Error("关闭缓存失败", zap.Error(err))
		} else {
			logger.Info("缓存系统已关闭")
		}
	}

	// 同步日志
	logger.Sync()

	logger.Info("优雅关闭流程完成")
}
