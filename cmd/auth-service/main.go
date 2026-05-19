package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"yuelaiengine/gateway/pkg/logger"
)

type userRecord struct {
	Password string
	UserID   string
	Role     string
}

type jwtClaims struct {
	Sub  string `json:"sub"`
	Role string `json:"role"`
	Exp  int64  `json:"exp"`
	Iat  int64  `json:"iat"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string `json:"token"`
	TokenType string `json:"token_type"`
	ExpiresIn int64  `json:"expires_in"`
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
}

type validateRequest struct {
	Token string `json:"token"`
}

type validateResponse struct {
	Valid  bool   `json:"valid"`
	UserID string `json:"user_id,omitempty"`
	Role   string `json:"role,omitempty"`
	Exp    int64  `json:"exp,omitempty"`
}

var (
	log logger.Logger
	users = map[string]userRecord{
		"admin":       {Password: "admin123", UserID: "u-admin", Role: "admin"},
		"hr":          {Password: "hr123", UserID: "u-hr", Role: "hr"},
		"interviewer": {Password: "interviewer123", UserID: "u-interviewer", Role: "interviewer"},
		"guest":       {Password: "guest123", UserID: "u-guest", Role: "guest"},
	}
)

func main() {
	var err error
	log, err = logger.NewWithConfigFile("./config/logs/auth-service-log.yaml")
	if err != nil {
		panic(err)
	}

	port := getPort("8085")
	secret := getEnv("JWT_SECRET", "your-very-secret-key-that-is-long-enough")
	ttl := getTTLSeconds(3600)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/login", loginHandler(secret, ttl))
	mux.HandleFunc("/validate", validateHandler(secret))

	server := &http.Server{
		Addr:         port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	ctx := context.Background()
	go func() {
		log.Info(ctx, "Starting Auth Service", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(ctx, "Could not start Auth Service", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info(ctx, "Auth Service is shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error(ctx, "Server shutdown error", "error", err)
	}
	log.Info(ctx, "Auth Service stopped")
}

func getPort(defaultPort string) string {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = defaultPort
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}
	return port
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getTTLSeconds(fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv("TOKEN_TTL_SECONDS"))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK\n"))
}

func loginHandler(secret string, ttlSeconds int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"code": "METHOD_NOT_ALLOWED", "message": "仅支持 POST"})
			return
		}

		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"code": "BAD_REQUEST", "message": "请求体格式错误"})
			return
		}

		user, ok := users[strings.TrimSpace(req.Username)]
		if !ok || user.Password != req.Password {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"code": "INVALID_CREDENTIALS", "message": "用户名或密码错误"})
			return
		}

		now := time.Now().Unix()
		exp := now + ttlSeconds
		claims := jwtClaims{Sub: user.UserID, Role: user.Role, Iat: now, Exp: exp}
		token, err := signJWT(claims, secret)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"code": "TOKEN_ISSUE_FAILED", "message": "生成令牌失败"})
			return
		}

		writeJSON(w, http.StatusOK, loginResponse{
			Token:     token,
			TokenType: "Bearer",
			ExpiresIn: ttlSeconds,
			UserID:    user.UserID,
			Role:      user.Role,
		})
	}
}

func validateHandler(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"code": "METHOD_NOT_ALLOWED", "message": "仅支持 POST"})
			return
		}

		token := extractBearer(r)
		if token == "" {
			var req validateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
				token = strings.TrimSpace(req.Token)
			}
		}
		if token == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"code": "TOKEN_MISSING", "message": "缺少访问令牌"})
			return
		}

		claims, err := verifyJWT(token, secret)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, validateResponse{Valid: false})
			return
		}

		writeJSON(w, http.StatusOK, validateResponse{
			Valid:  true,
			UserID: claims.Sub,
			Role:   claims.Role,
			Exp:    claims.Exp,
		})
	}
}

func extractBearer(r *http.Request) string {
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if authz == "" {
		return ""
	}
	parts := strings.SplitN(authz, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func signJWT(claims jwtClaims, secret string) (string, error) {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	headerPart := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadPart := base64.RawURLEncoding.EncodeToString(claimsJSON)
	unsigned := headerPart + "." + payloadPart
	signature := signHS256(unsigned, secret)
	return unsigned + "." + signature, nil
}

func verifyJWT(token, secret string) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("token 格式非法")
	}
	unsigned := parts[0] + "." + parts[1]
	expectedSig := signHS256(unsigned, secret)
	if !hmac.Equal([]byte(expectedSig), []byte(parts[2])) {
		return nil, fmt.Errorf("token 签名非法")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}

	var claims jwtClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, err
	}
	if claims.Exp <= time.Now().Unix() {
		return nil, fmt.Errorf("token 已过期")
	}
	if strings.TrimSpace(claims.Sub) == "" {
		return nil, fmt.Errorf("token 用户信息缺失")
	}
	return &claims, nil
}

func signHS256(content, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(content))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
