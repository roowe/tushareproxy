package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/roowe/tushareproxy/internal/cache"
	"github.com/roowe/tushareproxy/pkg/logger"

	"go.uber.org/zap"
)

// TushareAPIResult 用于检查API响应状态的简化结构体
type TushareAPIResult struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

const (
	TushareAPIURL = "http://api.waditu.com/dataapi"
)

// 全局缓存管理器
var cacheManager *cache.CacheManager

// SetCacheManager 设置缓存管理器
func SetCacheManager(cm *cache.CacheManager) {
	cacheManager = cm
}

// DataAPIHandler 处理/dataapi请求
func DataAPIHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// 设置响应头
	w.Header().Set("Content-Type", "application/json")

	// 只允许POST方法
	if r.Method != http.MethodPost {
		logger.Warn("不支持的HTTP方法", zap.String("method", r.Method))
		sendErrorResponse(w, "只支持POST方法", http.StatusMethodNotAllowed)
		return
	}

	// 读取请求体
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("读取请求体失败", zap.Error(err))
		sendErrorResponse(w, "读取请求体失败", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 生成缓存键
	var cacheKey string
	var response []byte
	var statusCode int
	var isFromCache bool

	if cacheManager != nil {
		cacheKey = cacheManager.GenerateKey(body)

		// 尝试从缓存获取
		if entry, found := cacheManager.Get(cacheKey); found {
			response = entry.ResponseBody
			statusCode = entry.StatusCode
			isFromCache = true
			logger.Info("使用缓存响应",
				zap.String("cache_key", cacheKey),
				zap.Int("status_code", statusCode))
		}
	}

	// 如果缓存未命中，转发请求
	if !isFromCache {
		logger.Info("转发tushare API请求", zap.String("body", string(body)))

		// 直接转发请求到tushare API
		var err error
		response, statusCode, err = forwardRawRequestToTushareAPI(body)
		if err != nil {
			logger.Error("转发请求到tushare API失败", zap.Error(err))
			sendErrorResponse(w, "请求tushare API失败", http.StatusInternalServerError)
			return
		}

		//logger.Info("tushare API响应", zap.Int("status_code", statusCode), zap.String("response", string(response)))

		// 解析响应，检查是否成功
		var shouldCache bool
		if statusCode == http.StatusOK && len(response) > 0 {
			var result TushareAPIResult
			if err := json.Unmarshal(response, &result); err == nil {
				if result.Code == 0 {
					shouldCache = true
					logger.Debug("tushare API响应成功，可以缓存", zap.Int("code", result.Code))
				} else {
					logger.Warn("tushare API返回错误码，不缓存",
						zap.Int("code", result.Code),
						zap.String("msg", result.Msg))
				}
			} else {
				logger.Error("解析tushare API响应失败", zap.Error(err))
			}
		}

		// 只有在响应成功且code=0时才缓存
		if cacheManager != nil && shouldCache {
			if err := cacheManager.Set(cacheKey, body, response, statusCode); err != nil {
				logger.Error("设置缓存失败", zap.Error(err))
				// 缓存失败不影响响应
			} else {
				logger.Debug("响应已缓存", zap.String("cache_key", cacheKey))
			}
		}
	}

	// 使用tushare返回的状态码
	w.WriteHeader(statusCode)
	if _, err := w.Write(response); err != nil {
		logger.Error("写入响应失败", zap.Error(err))
	}

	logger.Info("请求处理完成",
		zap.Duration("duration", time.Since(startTime)),
		zap.Bool("from_cache", isFromCache),
		zap.String("cache_key", cacheKey))
}

// forwardRawRequestToTushareAPI 直接转发原始请求到tushare API
func forwardRawRequestToTushareAPI(body []byte) ([]byte, int, error) {
	// 创建HTTP请求
	req, err := http.NewRequest("POST", TushareAPIURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, 0, fmt.Errorf("创建HTTP请求失败: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tushareproxy/1.0")

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("发送HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("读取响应失败: %w", err)
	}

	// 记录非200状态码
	if resp.StatusCode != http.StatusOK {
		logger.Warn("tushare API返回非200状态码",
			zap.Int("status_code", resp.StatusCode),
			zap.String("response", string(respBody)))
	}

	return respBody, resp.StatusCode, nil
}

// sendErrorResponse 发送错误响应
func sendErrorResponse(w http.ResponseWriter, message string, statusCode int) {
	w.WriteHeader(http.StatusOK) // 状态码固定为200

	errorResp := TushareAPIResult{
		Code: statusCode,
		Msg:  message,
	}

	response, _ := json.Marshal(errorResp)
	w.Write(response)
}
