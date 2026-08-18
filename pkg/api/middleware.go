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
// Logging middleware
// ============================================================================

// LoggingMiddleware logs each HTTP request's method, path, status code, and handling time
//
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response wrapper to capture the status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// handlerequest
		next.ServeHTTP(wrapped, r)

		// Log the request
		duration := time.Since(start)
		log.Printf("[API] %s %s %d %v",
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			duration,
		)
	})
}

// responseWriter is a response wrapper used to capture the status code
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
// Recovery middleware
// ============================================================================

// RecoveryMiddleware recovers from panics and returns a 500 error, preventing server crashes
//
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("[API] Panic recovered: %v", err)
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
// CORS middleware
// ============================================================================

// CORSMiddleware handles cross-origin resource sharing requests
//
func CORSMiddleware(config *Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Handle preflight requests
			if r.Method == http.MethodOptions {
				handleCORSPreflight(w, r, config)
				w.WriteHeader(http.StatusOK)
				return
			}

			// Set CORS headers
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

// handleCORSPreflight handles CORS preflight requests
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

// isOriginAllowed checks whether the origin is allowed
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

// joinStrings joins a string slice into a comma-separated string
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
// Rate limiting middleware
// ============================================================================

// RateLimitMiddleware rate-limits requests based on the token bucket algorithm
//
func RateLimitMiddleware(requestsPerSecond, burst int) func(http.Handler) http.Handler {
	if requestsPerSecond <= 0 {
		requestsPerSecond = 100
	}
	if burst <= 0 {
		burst = 200
	}

	// Create the token bucket
	rate := float64(requestsPerSecond)
	bucket := NewTokenBucket(rate, burst)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use the client IP as the rate limiting key
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

// TokenBucket implements the token bucket algorithm
type TokenBucket struct {
	mu       sync.Mutex
	tokens   float64
	rate     float64
	capacity int
	last     time.Time
}

// NewTokenBucket creates a new token bucket
func NewTokenBucket(rate float64, capacity int) *TokenBucket {
	return &TokenBucket{
		tokens:   float64(capacity),
		rate:     rate,
		capacity: capacity,
		last:     time.Now(),
	}
}

// Take attempts to acquire a token. Returns true on success, false if rate limited.
//
// key: used to track tokens for different clients
//
func (tb *TokenBucket) Take(key string) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()

	// Compute the number of tokens to refill
	elapsed := now.Sub(tb.last).Seconds()
	tb.tokens += elapsed * tb.rate
	if tb.tokens > float64(tb.capacity) {
		tb.tokens = float64(tb.capacity)
	}
	tb.last = now

	// Try to acquire a token
	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}

	return false
}

// ============================================================================
// Authentication middleware
// ============================================================================

// AuthMiddleware validates the API Key in the request
//
func AuthMiddleware(apiKeys []string) func(http.Handler) http.Handler {
	// Convert API keys into a hash set for fast lookup
	keySet := make(map[string]struct{})
	for _, key := range apiKeys {
		if key != "" {
			keySet[key] = struct{}{}
		}
	}

	// Precomputed set of hashed keys
	hashKeySet := make(map[string]struct{})
	for key := range keySet {
		hash := sha256.Sum256([]byte(key))
		hashKeySet[hex.EncodeToString(hash[:])] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Health check endpoint does not require authentication
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			// Read operations (GET) typically do not require authentication
			if r.Method == http.MethodGet {
				next.ServeHTTP(w, r)
				return
			}

			// Check for an API Key
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				// Also support passing it as a URL parameter
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

			// verify API Key
			if _, ok := keySet[apiKey]; !ok {
				// Also check the hashed version
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

// OptionalAuthMiddleware validates the API Key if provided; otherwise authentication is skipped
//
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

			// Validate the API Key if provided
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
// Helper functions
// ============================================================================

// getClientIP getclient IP address
func getClientIP(r *http.Request) string {
	// Check the X-Forwarded-For header first (reverse proxy scenario)
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		// X-Forwarded-For may contain multiple IPs; the first is the original client
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Check the X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Strip the port number
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == ':' {
			return ip[:i]
		}
	}
	return ip
}
