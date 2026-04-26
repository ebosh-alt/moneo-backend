package http

import (
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	envAppEnvironment = "APP_ENV"
	appEnvProduction  = "production"
)

type AuthRateLimitRule struct {
	MaxAttempts int
	Window      time.Duration
}

type AuthSecurityConfig struct {
	RequireHTTPSInProduction bool
	RateLimits               map[string]AuthRateLimitRule
	Logger                   *log.Logger
}

var defaultAuthRateLimits = map[string]AuthRateLimitRule{
	"/auth/login":                   {MaxAttempts: 5, Window: time.Minute},
	"/auth/register":                {MaxAttempts: 5, Window: time.Minute},
	"/auth/reset-password":          {MaxAttempts: 5, Window: time.Minute},
	"/auth/refresh":                 {MaxAttempts: 10, Window: time.Minute},
	"/auth/verify-email":            {MaxAttempts: 10, Window: time.Minute},
	"/auth/send-verification-email": {MaxAttempts: 5, Window: time.Minute},
	"/auth/forgot-password":         {MaxAttempts: 5, Window: time.Minute},
}

var authHTTPSRequiredPaths = map[string]struct{}{
	"/auth/login":                   {},
	"/auth/register":                {},
	"/auth/reset-password":          {},
	"/auth/refresh":                 {},
	"/auth/logout":                  {},
	"/auth/logout-all":              {},
	"/auth/verify-email":            {},
	"/auth/send-verification-email": {},
	"/auth/forgot-password":         {},
}

func DefaultAuthSecurityConfig() AuthSecurityConfig {
	return AuthSecurityConfig{
		RequireHTTPSInProduction: strings.EqualFold(strings.TrimSpace(os.Getenv(envAppEnvironment)), appEnvProduction),
		RateLimits:               cloneRateLimitRules(defaultAuthRateLimits),
	}
}

func NewAuthSecurityMiddleware(cfg AuthSecurityConfig) gin.HandlerFunc {
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}

	rateLimits := cfg.RateLimits
	if len(rateLimits) == 0 {
		rateLimits = cloneRateLimitRules(defaultAuthRateLimits)
	}

	limiter := newAuthRateLimiter(rateLimits, time.Now)

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		if cfg.RequireHTTPSInProduction && requiresHTTPS(path) && !isHTTPSRequest(c.Request) {
			writeSecurityEvent(logger, c, "auth_https_required", http.StatusBadRequest)
			c.AbortWithStatusJSON(http.StatusBadRequest, errorResponse{Error: "https_required"})
			return
		}

		allowed, retryAfter := limiter.Allow(path, requestClientKey(c))
		if !allowed {
			retryAfterSeconds := int(math.Ceil(retryAfter.Seconds()))
			if retryAfterSeconds < 1 {
				retryAfterSeconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
			writeSecurityEvent(logger, c, "auth_rate_limited", http.StatusTooManyRequests)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, errorResponse{Error: "rate_limited"})
			return
		}

		c.Next()

		if event, ok := suspiciousEvent(path, c.Writer.Status()); ok {
			writeSecurityEvent(logger, c, event, c.Writer.Status())
		}
	}
}

func requiresHTTPS(path string) bool {
	_, ok := authHTTPSRequiredPaths[path]
	return ok
}

func suspiciousEvent(path string, statusCode int) (string, bool) {
	switch path {
	case "/auth/login":
		if statusCode == http.StatusUnauthorized {
			return "auth_login_failed", true
		}
	case "/auth/refresh":
		if statusCode == http.StatusUnauthorized {
			return "auth_refresh_failed", true
		}
	case "/auth/reset-password":
		if statusCode == http.StatusUnauthorized {
			return "auth_reset_password_failed", true
		}
	case "/auth/verify-email":
		if statusCode == http.StatusUnauthorized {
			return "auth_verify_email_failed", true
		}
	case "/auth/register":
		if statusCode == http.StatusConflict {
			return "auth_register_duplicate_email", true
		}
	}

	return "", false
}

func writeSecurityEvent(logger *log.Logger, c *gin.Context, event string, statusCode int) {
	userAgent := strings.TrimSpace(c.Request.UserAgent())
	if len(userAgent) > 160 {
		userAgent = userAgent[:160]
	}

	logger.Printf(
		"security_event=%s method=%s path=%s ip=%s status=%d ua=%q",
		event,
		c.Request.Method,
		c.Request.URL.Path,
		requestClientKey(c),
		statusCode,
		userAgent,
	)
}

func requestClientKey(c *gin.Context) string {
	clientIP := strings.TrimSpace(c.ClientIP())
	if clientIP != "" {
		return clientIP
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(c.Request.RemoteAddr))
	if err == nil && host != "" {
		return host
	}

	return "unknown"
}

func isHTTPSRequest(request *http.Request) bool {
	if request == nil {
		return false
	}

	if request.TLS != nil {
		return true
	}

	if strings.EqualFold(firstHeaderToken(request.Header.Get("X-Forwarded-Proto")), "https") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(request.Header.Get("X-Forwarded-Ssl")), "on") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(request.Header.Get("X-Url-Scheme")), "https") {
		return true
	}

	return false
}

func firstHeaderToken(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}

	parts := strings.Split(value, ",")
	return strings.TrimSpace(parts[0])
}

func cloneRateLimitRules(source map[string]AuthRateLimitRule) map[string]AuthRateLimitRule {
	if len(source) == 0 {
		return nil
	}

	cloned := make(map[string]AuthRateLimitRule, len(source))
	for endpoint, rule := range source {
		cloned[endpoint] = rule
	}

	return cloned
}

type authRateLimiter struct {
	mu       sync.Mutex
	rules    map[string]AuthRateLimitRule
	counters map[string]rateLimitCounter
	now      func() time.Time
}

type rateLimitCounter struct {
	WindowStartedAt time.Time
	Count           int
}

func newAuthRateLimiter(rules map[string]AuthRateLimitRule, now func() time.Time) *authRateLimiter {
	clonedRules := cloneRateLimitRules(rules)
	if now == nil {
		now = time.Now
	}

	return &authRateLimiter{
		rules:    clonedRules,
		counters: make(map[string]rateLimitCounter, 128),
		now:      now,
	}
}

func (l *authRateLimiter) Allow(path string, clientKey string) (bool, time.Duration) {
	rule, limited := l.rules[path]
	if !limited || rule.MaxAttempts <= 0 || rule.Window <= 0 {
		return true, 0
	}

	now := l.now().UTC()
	bucket := path + "|" + clientKey

	l.mu.Lock()
	defer l.mu.Unlock()

	counter := l.counters[bucket]
	if counter.WindowStartedAt.IsZero() || now.Sub(counter.WindowStartedAt) >= rule.Window {
		counter = rateLimitCounter{
			WindowStartedAt: now,
			Count:           0,
		}
	}

	if counter.Count >= rule.MaxAttempts {
		retryAfter := rule.Window - now.Sub(counter.WindowStartedAt)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		l.counters[bucket] = counter
		return false, retryAfter
	}

	counter.Count++
	l.counters[bucket] = counter

	if len(l.counters) > 2048 {
		l.cleanupExpired(now)
	}

	return true, 0
}

func (l *authRateLimiter) cleanupExpired(now time.Time) {
	for bucket, counter := range l.counters {
		path := bucket
		if index := strings.Index(bucket, "|"); index >= 0 {
			path = bucket[:index]
		}

		rule, ok := l.rules[path]
		if !ok || rule.Window <= 0 {
			delete(l.counters, bucket)
			continue
		}

		if now.Sub(counter.WindowStartedAt) >= 2*rule.Window {
			delete(l.counters, bucket)
		}
	}
}
