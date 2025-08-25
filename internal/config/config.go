// package config
package config

import (
	"fmt"
	"os"
	"sync"

	"github.com/roowe/tushareproxy/pkg/logger"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// 主配置结构体
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	Tushare TushareConfig `mapstructure:"tushare"`
	Log     LogConfig     `mapstructure:"log"`
}

// 服务器配置
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// Tushare配置
type TushareConfig struct {
	Token string `mapstructure:"token"`
}

// 日志配置 - 直接使用 logger 包中的 Config 类型
type LogConfig = logger.Config

// 全局变量
var (
	globalConfig      *Config
	configMutex       sync.RWMutex
	watchers          []ConfigWatcher
	watcherMutex      sync.RWMutex
	viperInstance     *viper.Viper
	currentConfigPath string // 记住当前使用的配置文件路径
)

// 配置观察者接口
type ConfigWatcher interface {
	OnConfigChanged(*Config)
}

// 设置默认值
func setDefaultValues(v *viper.Viper) {
	// 服务器默认值
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 1155)

	// 日志默认值 - 直接使用 logger 包的默认配置
	logCfg := logger.DefaultConfig()
	v.SetDefault("log", logCfg)
}

// 验证配置
func validateConfig(config *Config) error {
	logger.Debug("validateConfig", zap.Any("config", config))

	// 验证服务器配置
	if config.Server.Host == "" {
		return fmt.Errorf("服务器主机地址不能为空")
	}
	if config.Server.Port < 1 || config.Server.Port > 65535 {
		return fmt.Errorf("无效的服务器端口: %d (端口范围: 1-65535)", config.Server.Port)
	}

	// 验证Tushare配置
	if config.Tushare.Token == "" {
		return fmt.Errorf("Tushare Token不能为空")
	}

	// 验证日志配置
	if config.Log.Level == "" {
		return fmt.Errorf("日志级别不能为空")
	}
	if config.Log.Format == "" {
		return fmt.Errorf("日志格式不能为空")
	}
	if config.Log.Output == "" {
		return fmt.Errorf("日志输出不能为空")
	}
	if config.Log.MaxSize <= 0 {
		return fmt.Errorf("无效的日志最大大小: %d", config.Log.MaxSize)
	}
	if config.Log.MaxAge <= 0 {
		return fmt.Errorf("无效的日志最大保留天数: %d", config.Log.MaxAge)
	}
	if config.Log.MaxBackups <= 0 {
		return fmt.Errorf("无效的日志最大备份数: %d", config.Log.MaxBackups)
	}

	return nil
}

// 加载配置的核心函数
func loadConfig(configPath string) (*Config, error) {
	v := viper.New()
	logger.Debug("configPath", zap.String("path", configPath))
	if configPath != "" {
		// 如果指定了配置文件路径，直接使用
		v.SetConfigFile(configPath)

		// 检查文件是否存在
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("指定的配置文件不存在: %s", configPath)
		}

		logger.Debug("使用指定配置文件", zap.String("path", configPath))
	} else {
		// 使用约定文件名方式
		v.SetConfigName("proxy")
		v.SetConfigType("toml")

		// 设置配置文件搜索路径
		v.AddConfigPath(".")
		v.AddConfigPath("./config")

		logger.Debug("搜索配置文件", zap.String("name", "proxy.toml"))
	}
	logger.Debug("read config file", zap.String("file", v.ConfigFileUsed()))
	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		if configPath != "" {
			return nil, fmt.Errorf("读取指定配置文件 %s 失败: %w", configPath, err)
		} else {
			logger.Error("read config file error", zap.Error(err))
			return nil, fmt.Errorf("未找到配置文件 proxy.toml，搜索路径: ./, ./config/")
		}
	}
	logger.Debug("read config file end")

	// 记录实际使用的配置文件
	logger.Info("成功加载配置文件", zap.String("file", v.ConfigFileUsed()))

	// 设置默认值
	setDefaultValues(v)

	// 解析配置到结构体
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 验证配置
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	// 保存viper实例用于后续使用
	viperInstance = v

	// 保存当前使用的配置文件路径
	currentConfigPath = configPath

	return &config, nil
}

// 简化的配置加载函数（使用默认约定方式）
func loadConfigDefault() (*Config, error) {
	return loadConfig("")
}

// 指定路径的配置加载函数
func loadConfigFromPath(configPath string) (*Config, error) {
	return loadConfig(configPath)
}

// 更新服务器端口配置
func UpdateServerPort(port int) {
	configMutex.Lock()
	defer configMutex.Unlock()
	if globalConfig != nil {
		globalConfig.Server.Port = port
	}
}

// 获取配置
func GetConfig() *Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return globalConfig
}

// 重新加载配置
func ReloadConfig() error {
	// 重新加载时使用相同的配置文件路径
	newConfig, err := loadConfig(currentConfigPath)
	if err != nil {
		return err
	}

	configMutex.Lock()
	globalConfig = newConfig
	configMutex.Unlock()

	// 通知所有观察者
	watcherMutex.RLock()
	for _, watcher := range watchers {
		go watcher.OnConfigChanged(newConfig)
	}
	watcherMutex.RUnlock()

	return nil
}

// 重新加载指定路径的配置
func ReloadConfigFromPath(configPath string) error {
	newConfig, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	configMutex.Lock()
	globalConfig = newConfig
	configMutex.Unlock()

	// 通知所有观察者
	watcherMutex.RLock()
	for _, watcher := range watchers {
		go watcher.OnConfigChanged(newConfig)
	}
	watcherMutex.RUnlock()

	return nil
}

// 初始化配置（使用默认约定方式）
func InitConfig() error {
	return InitConfigFromPath("")
}

// 初始化指定路径的配置
func InitConfigFromPath(configPath string) error {
	config, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	configMutex.Lock()
	globalConfig = config
	configMutex.Unlock()

	return nil
}

// 添加配置观察者
func WatchConfig(watcher ConfigWatcher) {
	watcherMutex.Lock()
	defer watcherMutex.Unlock()
	watchers = append(watchers, watcher)
}

// 获取当前使用的配置文件路径
func GetCurrentConfigPath() string {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return currentConfigPath
}

// 获取配置值（支持点号分隔的键）
func GetConfigValue(key string) interface{} {
	if viperInstance == nil {
		return nil
	}
	return viperInstance.Get(key)
}

// 获取字符串配置值
func GetConfigString(key string) string {
	if viperInstance == nil {
		return ""
	}
	return viperInstance.GetString(key)
}

// 获取整数配置值
func GetConfigInt(key string) int {
	if viperInstance == nil {
		return 0
	}
	return viperInstance.GetInt(key)
}

// 获取布尔配置值
func GetConfigBool(key string) bool {
	if viperInstance == nil {
		return false
	}
	return viperInstance.GetBool(key)
}

// 获取字符串切片配置值
func GetConfigStringSlice(key string) []string {
	if viperInstance == nil {
		return nil
	}
	return viperInstance.GetStringSlice(key)
}
