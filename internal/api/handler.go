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
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data *TushareAPIData `json:"data,omitempty"`
}

type TushareAPIData struct {
	Items []json.RawMessage `json:"items"`
}

const (
	TushareAPIURL = "http://api.waditu.com/dataapi"
)

const (
	cacheStatusHit      = "HIT"
	cacheStatusMiss     = "MISS"
	cacheStatusBypass   = "BYPASS"
	cacheStatusDisabled = "DISABLED"
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

	preparedRequest, err := parseIncomingRequest(body)
	if err != nil {
		logger.Warn("解析请求体失败", zap.Error(err))
		sendErrorResponse(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 生成缓存键
	var cacheKey string
	var namespace string
	var response []byte
	var statusCode int
	var isFromCache bool
	var cacheStatus = cacheStatusDisabled

	if cacheManager != nil {
		if err := preparedRequest.Policy.Validate(cacheManager.DefaultNamespace(), startTime); err != nil {
			logger.Warn("缓存策略校验失败", zap.Error(err))
			sendErrorResponse(w, err.Error(), http.StatusBadRequest)
			return
		}

		namespace = preparedRequest.Policy.ResolvedNamespace(cacheManager.DefaultNamespace())
		cacheKey = cacheManager.GenerateKey(namespace, preparedRequest.ForwardBody)
		cacheStatus = cacheStatusMiss

		if preparedRequest.Policy.NoCache {
			cacheStatus = cacheStatusBypass
		} else if entry, found := cacheManager.Get(cacheKey); found {
			response = entry.ResponseBody
			statusCode = entry.StatusCode
			isFromCache = true
			cacheStatus = cacheStatusHit
			logger.Info("使用缓存响应",
				zap.String("api_name", preparedRequest.APIName),
				zap.String("cache_key", cacheKey),
				zap.String("namespace", namespace),
				zap.Int("status_code", statusCode))
		}
	}

	// 如果缓存未命中，转发请求
	if !isFromCache {
		logger.Info("转发tushare API请求",
			zap.String("api_name", preparedRequest.APIName),
			zap.String("namespace", namespace),
			zap.String("cache_status", cacheStatus),
			zap.Bool("no_cache", preparedRequest.Policy.NoCache))

		// 直接转发请求到tushare API
		var err error
		response, statusCode, err = forwardRawRequestToTushareAPI(preparedRequest.ForwardBody)
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
					itemCount := result.itemCount()
					if itemCount > 0 {
						shouldCache = true
						logger.Debug("tushare API响应成功，可以缓存",
							zap.Int("code", result.Code),
							zap.Int("item_count", itemCount))
					} else {
						logger.Info("tushare API响应成功但无数据，不缓存",
							zap.Int("code", result.Code),
							zap.Int("item_count", itemCount))
					}
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
		if cacheManager != nil && shouldCache && !preparedRequest.Policy.NoCache {
			cacheExpiresAt, err := resolveCacheExpiration(
				preparedRequest.Policy,
				cacheManager.DefaultTTL(),
				time.Now(),
			)
			if err != nil {
				logger.Error("解析缓存过期时间失败", zap.Error(err))
			} else if err := cacheManager.Set(
				cacheKey,
				namespace,
				preparedRequest.ForwardBody,
				response,
				statusCode,
				cacheExpiresAt,
			); err != nil {
				logger.Error("设置缓存失败", zap.Error(err))
				// 缓存失败不影响响应
			} else {
				logger.Debug("响应已缓存",
					zap.String("cache_key", cacheKey),
					zap.String("namespace", namespace),
					zap.Int64("expires_at", cacheExpiresAt.Unix()))
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
		zap.String("cache_status", cacheStatus),
		zap.String("namespace", namespace),
		zap.String("cache_key", cacheKey),
		zap.String("api_name", preparedRequest.APIName))
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

func (r TushareAPIResult) itemCount() int {
	if r.Data == nil {
		return 0
	}
	return len(r.Data.Items)
}
