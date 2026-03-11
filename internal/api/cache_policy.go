package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

var cacheNamespacePattern = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)

const maxUnixTimestampSeconds int64 = 9999999999

// CachePolicy 表示请求级缓存控制策略。
type CachePolicy struct {
	Namespace string `json:"namespace,omitempty"`
	TTL       *int64 `json:"ttl,omitempty"`
	ExpiresAt *int64 `json:"expires_at,omitempty"`
	NoCache   bool   `json:"no_cache,omitempty"`
}

// PreparedRequest 表示剥离 _cache 后可转发的请求。
type PreparedRequest struct {
	ForwardBody []byte
	Policy      CachePolicy
	APIName     string
}

func parseIncomingRequest(body []byte) (*PreparedRequest, error) {
	trimmedBody := bytes.TrimSpace(body)
	if len(trimmedBody) == 0 {
		return nil, fmt.Errorf("请求体不能为空")
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmedBody))
	decoder.UseNumber()

	var payload map[string]interface{}
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("请求体必须是合法 JSON 对象: %w", err)
	}
	if payload == nil {
		return nil, fmt.Errorf("请求体必须是 JSON 对象")
	}
	if err := ensureSingleJSONObject(decoder); err != nil {
		return nil, err
	}

	prepared := &PreparedRequest{}
	if apiName, ok := payload["api_name"].(string); ok {
		prepared.APIName = strings.TrimSpace(apiName)
	}

	if rawPolicy, ok := payload["_cache"]; ok {
		if rawPolicy != nil {
			policyBytes, err := json.Marshal(rawPolicy)
			if err != nil {
				return nil, fmt.Errorf("序列化 _cache 失败: %w", err)
			}

			policyDecoder := json.NewDecoder(bytes.NewReader(policyBytes))
			policyDecoder.DisallowUnknownFields()
			if err := policyDecoder.Decode(&prepared.Policy); err != nil {
				return nil, fmt.Errorf("_cache 字段非法: %w", err)
			}
		}
		delete(payload, "_cache")
	}

	sanitizedBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化请求体失败: %w", err)
	}

	prepared.ForwardBody = sanitizedBody
	return prepared, nil
}

func ensureSingleJSONObject(decoder *json.Decoder) error {
	var extra interface{}
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("请求体解析失败: %w", err)
	}
	return fmt.Errorf("请求体只能包含一个 JSON 对象")
}

func (p CachePolicy) ResolvedNamespace(defaultNamespace string) string {
	namespace := strings.TrimSpace(p.Namespace)
	if namespace != "" {
		return namespace
	}

	defaultNamespace = strings.TrimSpace(defaultNamespace)
	if defaultNamespace == "" {
		return "default"
	}
	return defaultNamespace
}

func (p CachePolicy) Validate(defaultNamespace string, now time.Time) error {
	namespace := p.ResolvedNamespace(defaultNamespace)
	if !cacheNamespacePattern.MatchString(namespace) {
		return fmt.Errorf("namespace 只能包含字母、数字、点、下划线、短横线和冒号")
	}

	if p.TTL != nil && *p.TTL <= 0 {
		return fmt.Errorf("ttl 必须大于 0")
	}

	if p.ExpiresAt != nil {
		if *p.ExpiresAt <= 0 {
			return fmt.Errorf("expires_at 必须大于 0")
		}
		if *p.ExpiresAt > maxUnixTimestampSeconds {
			return fmt.Errorf("expires_at 必须是秒级 Unix 时间戳，不支持毫秒")
		}
		if !time.Unix(*p.ExpiresAt, 0).After(now) {
			return fmt.Errorf("expires_at 必须晚于当前时间")
		}
	}

	return nil
}

func resolveCacheExpiration(
	policy CachePolicy,
	defaultTTL time.Duration,
	now time.Time,
) (time.Time, error) {
	if defaultTTL <= 0 {
		return time.Time{}, fmt.Errorf("默认缓存 TTL 必须大于 0")
	}

	var resolvedExpiration time.Time

	if policy.TTL != nil {
		resolvedExpiration = now.Add(time.Duration(*policy.TTL) * time.Second)
	}

	if policy.ExpiresAt != nil {
		expiresAt := time.Unix(*policy.ExpiresAt, 0)
		if resolvedExpiration.IsZero() || expiresAt.Before(resolvedExpiration) {
			resolvedExpiration = expiresAt
		}
	}

	if resolvedExpiration.IsZero() {
		resolvedExpiration = now.Add(defaultTTL)
	}

	return resolvedExpiration, nil
}
