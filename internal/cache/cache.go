package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/roowe/tushareproxy/pkg/logger"
	"go.uber.org/zap"
)

// CacheManager 缓存管理器
type CacheManager struct {
	db  *badger.DB
	ttl time.Duration
}

// CacheEntry 缓存条目
type CacheEntry struct {
	RequestBody  []byte `json:"request_body"`
	ResponseBody []byte `json:"response_body"`
	StatusCode   int    `json:"status_code"`
	Timestamp    int64  `json:"timestamp"`
}

// NewCacheManager 创建新的缓存管理器
func NewCacheManager(dbPath string, ttlDays int) (*CacheManager, error) {
	// 配置BadgerDB选项
	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil // 禁用BadgerDB的默认日志输出

	// 打开数据库
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("打开BadgerDB失败: %w", err)
	}

	ttl := time.Duration(ttlDays) * 24 * time.Hour

	logger.Info("缓存管理器初始化成功",
		zap.String("db_path", dbPath),
		zap.Int("ttl_days", ttlDays))

	return &CacheManager{
		db:  db,
		ttl: ttl,
	}, nil
}

// Close 关闭缓存管理器
func (cm *CacheManager) Close() error {
	if cm.db != nil {
		logger.Info("正在关闭缓存数据库")
		return cm.db.Close()
	}
	return nil
}

// GenerateKey 根据请求体生成缓存键
func (cm *CacheManager) GenerateKey(requestBody []byte) string {
	hash := sha256.Sum256(requestBody)
	return hex.EncodeToString(hash[:])
}

// Get 从缓存中获取数据
func (cm *CacheManager) Get(key string) (*CacheEntry, bool) {
	var entry *CacheEntry

	err := cm.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &entry)
		})
	})

	if err != nil {
		if err == badger.ErrKeyNotFound {
			logger.Debug("缓存未命中", zap.String("key", key))
		} else {
			logger.Error("从缓存读取数据失败", zap.Error(err), zap.String("key", key))
		}
		return nil, false
	}

	// 检查是否过期（额外的过期检查，虽然BadgerDB会自动处理TTL）
	if time.Since(time.Unix(entry.Timestamp, 0)) > cm.ttl {
		logger.Debug("缓存已过期", zap.String("key", key))
		cm.Delete(key) // 异步删除过期的条目
		return nil, false
	}

	logger.Debug("缓存命中", zap.String("key", key))
	return entry, true
}

// Set 设置缓存数据
func (cm *CacheManager) Set(key string, requestBody, responseBody []byte, statusCode int) error {
	entry := &CacheEntry{
		RequestBody:  requestBody,
		ResponseBody: responseBody,
		StatusCode:   statusCode,
		Timestamp:    time.Now().Unix(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("序列化缓存条目失败: %w", err)
	}

	err = cm.db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry([]byte(key), data).WithTTL(cm.ttl)
		return txn.SetEntry(e)
	})

	if err != nil {
		logger.Error("设置缓存失败", zap.Error(err), zap.String("key", key))
		return fmt.Errorf("设置缓存失败: %w", err)
	}

	logger.Debug("缓存设置成功",
		zap.String("key", key),
		zap.Int("status_code", statusCode),
		zap.Int("response_size", len(responseBody)))

	return nil
}

// Delete 删除缓存条目
func (cm *CacheManager) Delete(key string) error {
	err := cm.db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})

	if err != nil && err != badger.ErrKeyNotFound {
		logger.Error("删除缓存失败", zap.Error(err), zap.String("key", key))
		return fmt.Errorf("删除缓存失败: %w", err)
	}

	return nil
}

// GetStats 获取缓存统计信息
func (cm *CacheManager) GetStats() map[string]interface{} {
	lsm, vlog := cm.db.Size()

	stats := map[string]interface{}{
		"lsm_size":   lsm,
		"vlog_size":  vlog,
		"total_size": lsm + vlog,
	}

	return stats
}

// RunGC 运行垃圾回收
func (cm *CacheManager) RunGC() error {
	logger.Info("开始运行缓存垃圾回收")
	logger.Info("缓存 stats", zap.Any("stats", cm.GetStats()))

	err := cm.db.RunValueLogGC(0.5)
	if err != nil && err != badger.ErrNoRewrite {
		logger.Error("垃圾回收失败", zap.Error(err))
		return err
	}

	logger.Info("缓存垃圾回收完成")
	logger.Info("缓存 stats", zap.Any("stats", cm.GetStats()))

	return nil
}

// StartGCRoutine 启动后台垃圾回收例程
func (cm *CacheManager) StartGCRoutine() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			cm.RunGC()
		}
	}()

	logger.Info("缓存垃圾回收例程已启动")
}
