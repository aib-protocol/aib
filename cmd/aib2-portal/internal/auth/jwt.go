package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// JWTAuth handles simple JWT-like authentication for admin routes
type JWTAuth struct {
	secretKey string
	mu        sync.RWMutex
	sessions  map[string]session
}

type session struct {
	username  string
	expiresAt time.Time
}

// New creates a new JWTAuth instance
func New() *JWTAuth {
	secret := os.Getenv("ADMIN_JWT_SECRET")
	if secret == "" {
		// Generate a random secret for development
		b := make([]byte, 32)
		rand.Read(b)
		secret = base64.StdEncoding.EncodeToString(b)
	}

	return &JWTAuth{
		secretKey: secret,
		sessions:  make(map[string]session),
	}
}

// GenerateToken creates a new session token for a user
func (j *JWTAuth) GenerateToken(username string) (string, error) {
	// Generate random token
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.StdEncoding.EncodeToString(b)

	j.mu.Lock()
	j.sessions[token] = session{
		username:  username,
		expiresAt: time.Now().Add(24 * time.Hour),
	}
	j.mu.Unlock()

	return token, nil
}

// ValidateToken checks if a token is valid
func (j *JWTAuth) ValidateToken(token string) (string, bool) {
	j.mu.RLock()
	session, exists := j.sessions[token]
	j.mu.RUnlock()

	if !exists {
		return "", false
	}

	if time.Now().After(session.expiresAt) {
		j.mu.Lock()
		delete(j.sessions, token)
		j.mu.Unlock()
		return "", false
	}

	return session.username, true
}

// InvalidateToken removes a token
func (j *JWTAuth) InvalidateToken(token string) {
	j.mu.Lock()
	delete(j.sessions, token)
	j.mu.Unlock()
}

// GetCredentials returns the configured admin credentials
func GetCredentials() (string, string) {
	username := os.Getenv("ADMIN_USERNAME")
	password := os.Getenv("ADMIN_PASSWORD")

	if username == "" {
		username = "admin"
	}
	if password == "" {
		panic("ADMIN_PASSWORD environment variable must be set; refusing to start with a default password")
	}

	return username, password
}

// Middleware returns an authentication middleware for admin routes
func (j *JWTAuth) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get token from Authorization header or cookie
		token := ""
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			cookie, err := r.Cookie("admin_token")
			if err == nil {
				token = cookie.Value
			}
		}

		if token == "" {
			j.unauthorized(w)
			return
		}

		username, valid := j.ValidateToken(token)
		if !valid {
			j.unauthorized(w)
			return
		}

		// Add username to request context (for future use)
		r.Header.Set("X-Admin-User", username)
		next(w, r)
	}
}

func (j *JWTAuth) unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintf(w, `{"error":"unauthorized","message":"Please login first"}`)
}

// Unauthorized is a public method for unauthorized responses
func (j *JWTAuth) Unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintf(w, `{"error":"unauthorized","message":"Please login first"}`)
}

// LoginHandler handles login requests
func (j *JWTAuth) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	expectedUsername, expectedPassword := GetCredentials()

	if creds.Username != expectedUsername || creds.Password != expectedPassword {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error":"invalid_credentials","message":"Invalid username or password"}`)
		return
	}

	token, err := j.GenerateToken(creds.Username)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_token",
		Value:    token,
		Path:     "/admin/",
		MaxAge:   86400, // 24 hours
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"success":true,"token":"%s","message":"Login successful"}`, token)
}

// LogoutHandler handles logout requests
func (j *JWTAuth) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get token from cookie
	cookie, err := r.Cookie("admin_token")
	if err == nil {
		j.InvalidateToken(cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_token",
		Value:    "",
		Path:     "/admin/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"success":true,"message":"Logout successful"}`)
}
