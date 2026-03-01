package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// ============================================================================
// 日志中间件
// ============================================================================

// LoggingMiddleware 日志中间件
//
// 记录每个 HTTP 请求的方法、路径、状态码和处理时间
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 创建响应包装器来捕获状态码
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:    http.StatusOK,
		}

		// 处理请求
		next.ServeHTTP(wrapped, r)

		// 记录日志
		duration := time.Since(start)
		log.Printf("[API] %s %s %d %v",
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			duration,
		)
	})
}

// responseWriter 响应包装器，用于捕获状态码
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.ResponseWriter.WriteHeader(code)
		rw.written = true
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// ============================================================================
// 恢复中间件
// ============================================================================

// RecoveryMiddleware 恢复中间件
//
// 捕获 panic 并返回 500 错误，防止服务器崩溃
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("[API] Panic 恢复: %v", err)
				writeError(w, http.StatusInternalServerError,
					ErrCodeInternalError,
					"Internal server error",
					fmt.Sprintf("%v", err),
				)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ============================================================================
// CORS 中间件
// ============================================================================

// CORSMiddleware CORS 中间件
//
// 处理跨域资源共享请求
func CORSMiddleware(config *Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 预检请求处理
			if r.Method == http.MethodOptions {
				handleCORSPreflight(w, r, config)
				w.WriteHeader(http.StatusOK)
				return
			}

			// 设置 CORS 头
			origin := r.Header.Get("Origin")
			if isOriginAllowed(origin, config.AllowedOrigins) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			} else if len(config.AllowedOrigins) > 0 && config.AllowedOrigins[0] == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			}

			w.Header().Set("Access-Control-Allow-Methods", joinStrings(config.AllowedMethods))
			w.Header().Set("Access-Control-Allow-Headers", joinStrings(config.AllowedHeaders))
			w.Header().Set("Access-Control-Max-Age", "86400")

			next.ServeHTTP(w, r)
		})
	}
}

// handleCORSPreflight 处理 CORS 预检请求
func handleCORSPreflight(w http.ResponseWriter, r *http.Request, config *Config) {
	origin := r.Header.Get("Origin")
	if isOriginAllowed(origin, config.AllowedOrigins) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else if len(config.AllowedOrigins) > 0 && config.AllowedOrigins[0] == "*" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}

	w.Header().Set("Access-Control-Allow-Methods", joinStrings(config.AllowedMethods))
	w.Header().Set("Access-Control-Allow-Headers", joinStrings(config.AllowedHeaders))
	w.Header().Set("Access-Control-Max-Age", "86400")
}

// isOriginAllowed 检查 origin 是否被允许
func isOriginAllowed(origin string, allowed []string) bool {
	if origin == "" {
		return false
	}

	for _, o := range allowed {
		if o == origin {
			return true
		}
	}
	return false
}

// joinStrings 将字符串切片连接为逗号分隔的字符串
func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}

// ============================================================================
// 限流中间件
// ============================================================================

// RateLimitMiddleware 限流中间件
//
// 基于令牌桶算法的请求限流
func RateLimitMiddleware(requestsPerSecond, burst int) func(http.Handler) http.Handler {
	if requestsPerSecond <= 0 {
		requestsPerSecond = 100
	}
	if burst <= 0 {
		burst = 200
	}

	// 创建令牌桶
	rate := float64(requestsPerSecond)
	bucket := NewTokenBucket(rate, burst)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 使用客户端 IP 作为限流键
			ip := getClientIP(r)

			if !bucket.Take(ip) {
				writeError(w, http.StatusTooManyRequests,
					ErrCodeRateLimited,
					"Rate limit exceeded",
					fmt.Sprintf("Maximum %d requests per second", requestsPerSecond),
				)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TokenBucket 令牌桶实现
type TokenBucket struct {
	mu      sync.Mutex
	tokens  float64
	rate    float64
	capacity int
	last    time.Time
}

// NewTokenBucket 创建新的令牌桶
func NewTokenBucket(rate float64, capacity int) *TokenBucket {
	return &TokenBucket{
		tokens:   float64(capacity),
		rate:     rate,
		capacity: capacity,
		last:     time.Now(),
	}
}

// Take 尝试获取一个令牌
//
// key: 用于跟踪不同客户端的令牌
// 返回 true 表示获取成功，false 表示被限流
func (tb *TokenBucket) Take(key string) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()

	// 计算应该补充的令牌数
	elapsed := now.Sub(tb.last).Seconds()
	tb.tokens += elapsed * tb.rate
	if tb.tokens > float64(tb.capacity) {
		tb.tokens = float64(tb.capacity)
	}
	tb.last = now

	// 尝试获取令牌
	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}

	return false
}

// ============================================================================
// 认证中间件
// ============================================================================

// AuthMiddleware 认证中间件
//
// 验证请求中的 API Key
func AuthMiddleware(apiKeys []string) func(http.Handler) http.Handler {
	// 将 API keys 转换为哈希集合以便快速查找
	keySet := make(map[string]struct{})
	for _, key := range apiKeys {
		if key != "" {
			keySet[key] = struct{}{}
		}
	}

	// 预计算的哈希 key 集合
	hashKeySet := make(map[string]struct{})
	for key := range keySet {
		hash := sha256.Sum256([]byte(key))
		hashKeySet[hex.EncodeToString(hash[:])] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 健康检查端点不需要认证
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			// 读操作（GET）通常不需要认证
			if r.Method == http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}

			// 检查是否有 API Key
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				// 也支持在 URL 参数中传递
				apiKey = r.URL.Query().Get("api_key")
			}

			if apiKey == "" {
				writeError(w, http.StatusUnauthorized,
					ErrCodeUnauthorized,
					"Missing API key",
					"Provide API key in X-API-Key header or api_key query parameter",
				)
				return
			}

			// 验证 API Key
			if _, ok := keySet[apiKey]; !ok {
				// 也检查哈希版本
				hash := sha256.Sum256([]byte(apiKey))
				hashStr := hex.EncodeToString(hash[:])
				if _, ok := hashKeySet[hashStr]; !ok {
					writeError(w, http.StatusUnauthorized,
						ErrCodeUnauthorized,
						"Invalid API key",
						"The provided API key is not valid",
					)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// OptionalAuthMiddleware 可选认证中间件
//
// 如果提供了 API Key 则验证，否则跳过认证
func OptionalAuthMiddleware(apiKeys []string) func(http.Handler) http.Handler {
	keySet := make(map[string]struct{})
	for _, key := range apiKeys {
		if key != "" {
			keySet[key] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				apiKey = r.URL.Query().Get("api_key")
			}

			// 如果提供了 API Key 则验证
			if apiKey != "" {
				if _, ok := keySet[apiKey]; !ok {
					writeError(w, http.StatusUnauthorized,
						ErrCodeUnauthorized,
						"Invalid API key",
						"The provided API key is not valid",
					)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// ============================================================================
// 辅助函数
// ============================================================================

// getClientIP 获取客户端 IP 地址
func getClientIP(r *http.Request) string {
	// 优先检查 X-Forwarded-For 头（反向代理场景）
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// X-Forwarded-For 可能包含多个 IP，第一个是原始客户端
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// 检查 X-Real-IP 头
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// 回退到 RemoteAddr
	ip := r.RemoteAddr
	// 移除端口号
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == ':' {
			return ip[:i]
		}
	}
	return ip
}
