package main

import (
	"github.com/roowe/tushareproxy/internal/config"

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

}

// 设置优雅关闭
func setupGracefulShutdown() {
	// 创建信号通道
	sigChan := make(chan os.Signal, 1)

	// 监听系统信号
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 在后台处理信号
	go func() {
		sig := <-sigChan
		logger.Info("收到关闭信号，开始优雅关闭", zap.String("signal", sig.String()))

		// 执行优雅关闭流程
		gracefulShutdown()

		// 退出程序
		os.Exit(0)
	}()
}

// 优雅关闭流程
func gracefulShutdown() {
	logger.Info("开始优雅关闭流程")

	// if wsServer != nil {
	// 	logger.Info("正在停止网络服务器")
	// 	if err := wsServer.Stop(); err != nil {
	// 		logger.Error("停止网络服务器失败", zap.Error(err))
	// 	} else {
	// 		logger.Info("网络服务器已停止")
	// 	}
	// }

	// 3. 同步日志
	logger.Sync()

	logger.Info("优雅关闭流程完成")
}
