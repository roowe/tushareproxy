package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	globalLogger *zap.Logger
	mu           sync.RWMutex
	initialized  bool
)

// Config 日志配置
type Config struct {
	Level      string `json:"level" mapstructure:"level"`              // 日志级别: debug, info, warn, error
	Format     string `json:"format"  mapstructure:"format"`           // 日志格式: json, console
	Output     string `json:"output"  mapstructure:"output"`           // 输出方式: console, file, both
	FilePath   string `json:"file_path"  mapstructure:"file_path"`     // 日志文件路径
	MaxSize    int    `json:"max_size" mapstructure:"max_size"`        // 单个日志文件最大大小(MB)
	MaxBackups int    `json:"max_backups"  mapstructure:"max_backups"` // 最大备份文件数
	MaxAge     int    `json:"max_age" mapstructure:"max_age"`          // 日志文件最大保存天数
	Compress   bool   `json:"compress" mapstructure:"compress"`        // 是否压缩备份文件
}

// DefaultConfig 默认配置
func DefaultConfig() *Config {
	return &Config{
		Level:      "debug",
		Format:     "console",
		Output:     "console",
		FilePath:   "/tmp/njjgo/logs/app.log",
		MaxSize:    100,
		MaxBackups: 3,
		MaxAge:     7,
		Compress:   false,
	}
}

// InitDefaultLogger 初始化默认日志器
func InitDefaultLogger() error {
	return InitLogger(DefaultConfig())
}

// InitLogger 初始化日志（支持重新配置）
func InitLogger(cfg *Config) error {
	mu.Lock()
	defer mu.Unlock()

	if cfg == nil {
		cfg = DefaultConfig()
	}

	// 设置日志级别
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return fmt.Errorf("解析日志级别失败: %v", err)
	}

	// 创建编码器配置
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	// 选择编码器
	var encoder zapcore.Encoder
	switch cfg.Format {
	case "json":
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	default:
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// 创建核心
	var cores []zapcore.Core

	// 控制台输出
	if cfg.Output == "console" || cfg.Output == "both" {
		consoleCore := zapcore.NewCore(
			encoder,
			zapcore.AddSync(os.Stdout),
			level,
		)
		cores = append(cores, consoleCore)
	}

	// 文件输出
	if cfg.Output == "file" || cfg.Output == "both" {
		// 确保日志目录存在
		if err := os.MkdirAll(filepath.Dir(cfg.FilePath), 0755); err != nil {
			return fmt.Errorf("创建日志目录失败: %v", err)
		}

		// 配置日志轮转
		writer := &lumberjack.Logger{
			Filename:   cfg.FilePath,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}

		fileCore := zapcore.NewCore(
			encoder,
			zapcore.AddSync(writer),
			level,
		)
		cores = append(cores, fileCore)
	}

	if len(cores) == 0 {
		return fmt.Errorf("未配置任何日志输出方式")
	}

	// 创建核心
	core := zapcore.NewTee(cores...)

	// 如果已经有logger，先同步并关闭
	if globalLogger != nil {
		globalLogger.Sync()
	}

	// 创建新的 logger
	globalLogger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	initialized = true

	// 输出启动信息 - 使用 Skip(1) 来跳过当前函数，显示正确的调用位置
	globalLogger.WithOptions(zap.AddCallerSkip(1)).Info("日志系统初始化完成",
		zap.String("level", cfg.Level),
		zap.String("format", cfg.Format),
		zap.String("output", cfg.Output))

	return nil
}

// ReconfigureLogger 重新配置日志器
func ReconfigureLogger(cfg *Config) error {
	if !initialized {
		return fmt.Errorf("日志器尚未初始化，请先调用 InitDefaultLogger() 或 InitLogger()")
	}
	return InitLogger(cfg)
}

// GetLogger 获取全局 logger（线程安全）
func GetLogger() *zap.Logger {
	mu.RLock()
	defer mu.RUnlock()

	if globalLogger == nil {
		// 如果没有初始化，使用默认配置
		if err := InitDefaultLogger(); err != nil {
			panic(fmt.Sprintf("初始化默认日志失败: %v", err))
		}
	}
	return globalLogger
}

// Sync 同步日志
func Sync() error {
	mu.RLock()
	defer mu.RUnlock()

	if globalLogger != nil {
		return globalLogger.Sync()
	}
	return nil
}

// IsInitialized 检查日志器是否已初始化
func IsInitialized() bool {
	mu.RLock()
	defer mu.RUnlock()
	return initialized
}

// 便捷方法，直接调用全局 logger
func Debug(msg string, fields ...zap.Field) {
	GetLogger().WithOptions(zap.AddCallerSkip(1)).Debug(msg, fields...)
}

func Info(msg string, fields ...zap.Field) {
	GetLogger().WithOptions(zap.AddCallerSkip(1)).Info(msg, fields...)
}

func Warn(msg string, fields ...zap.Field) {
	GetLogger().WithOptions(zap.AddCallerSkip(1)).Warn(msg, fields...)
}

func Error(msg string, fields ...zap.Field) {
	GetLogger().WithOptions(zap.AddCallerSkip(1)).Error(msg, fields...)
}

func Fatal(msg string, fields ...zap.Field) {
	GetLogger().WithOptions(zap.AddCallerSkip(1)).Fatal(msg, fields...)
}

func Panic(msg string, fields ...zap.Field) {
	GetLogger().WithOptions(zap.AddCallerSkip(1)).Panic(msg, fields...)
}

// 带上下文的便捷方法
func With(fields ...zap.Field) *zap.Logger {
	return GetLogger().WithOptions(zap.AddCallerSkip(1)).With(fields...)
}

// 设置环境变量来配置日志
func init() {
	// 如果环境变量设置了日志级别，自动初始化
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		cfg := DefaultConfig()
		cfg.Level = level
		if err := InitLogger(cfg); err != nil {
			// 初始化失败时不 panic，让程序继续运行
			fmt.Printf("自动初始化日志失败: %v\n", err)
		}
	}
}
