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
	appEnvProd        = "prod"
)

type AuthRateLimitRule struct {
	MaxAttempts int
	Window      time.Duration
}

type AuthSecurityConfig struct {
	RequireHTTPSInProduction bool
	TrustForwardedHeaders    bool
	TrustedProxyCIDRs        []string
	RateLimits               map[string]AuthRateLimitRule
	Logger                   *log.Logger
}

var defaultAuthRateLimits = map[string]AuthRateLimitRule{
	"/api/v1/auth/login":                   {MaxAttempts: 5, Window: time.Minute},
	"/api/v1/auth/register":                {MaxAttempts: 5, Window: time.Minute},
	"/api/v1/auth/reset-password":          {MaxAttempts: 5, Window: time.Minute},
	"/api/v1/auth/refresh":                 {MaxAttempts: 10, Window: time.Minute},
	"/api/v1/auth/verify-email":            {MaxAttempts: 10, Window: time.Minute},
	"/api/v1/auth/send-verification-email": {MaxAttempts: 5, Window: time.Minute},
	"/api/v1/auth/forgot-password":         {MaxAttempts: 5, Window: time.Minute},
}

var authHTTPSRequiredPaths = map[string]struct{}{
	"/api/v1/auth/login":                   {},
	"/api/v1/auth/register":                {},
	"/api/v1/auth/reset-password":          {},
	"/api/v1/auth/refresh":                 {},
	"/api/v1/auth/logout":                  {},
	"/api/v1/auth/logout-all":              {},
	"/api/v1/auth/verify-email":            {},
	"/api/v1/auth/send-verification-email": {},
	"/api/v1/auth/forgot-password":         {},
}

func DefaultAuthSecurityConfig() AuthSecurityConfig {
	return AuthSecurityConfig{
		RequireHTTPSInProduction: isProductionEnvironment(strings.TrimSpace(os.Getenv(envAppEnvironment))),
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

	trustedProxyNets := parseTrustedProxyNetworks(cfg.TrustedProxyCIDRs)
	limiter := newAuthRateLimiter(rateLimits, time.Now)

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}

		clientKey := requestClientKey(c.Request, cfg.TrustForwardedHeaders, trustedProxyNets)
		if cfg.RequireHTTPSInProduction && requiresHTTPS(path) && !isHTTPSRequest(c.Request, cfg.TrustForwardedHeaders, trustedProxyNets) {
			writeSecurityEvent(logger, c, clientKey, "auth_https_required", http.StatusBadRequest)
			c.AbortWithStatusJSON(http.StatusBadRequest, errorResponse{Error: "https_required"})
			return
		}

		allowed, retryAfter := limiter.Allow(path, clientKey)
		if !allowed {
			retryAfterSeconds := int(math.Ceil(retryAfter.Seconds()))
			if retryAfterSeconds < 1 {
				retryAfterSeconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(retryAfterSeconds))
			writeSecurityEvent(logger, c, clientKey, "auth_rate_limited", http.StatusTooManyRequests)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, errorResponse{Error: "rate_limited"})
			return
		}

		c.Next()

		if event, ok := suspiciousEvent(path, c.Writer.Status()); ok {
			writeSecurityEvent(logger, c, clientKey, event, c.Writer.Status())
		}
	}
}

func requiresHTTPS(path string) bool {
	_, ok := authHTTPSRequiredPaths[path]
	return ok
}

func suspiciousEvent(path string, statusCode int) (string, bool) {
	switch path {
	case "/api/v1/auth/login":
		if statusCode == http.StatusUnauthorized {
			return "auth_login_failed", true
		}
	case "/api/v1/auth/refresh":
		if statusCode == http.StatusUnauthorized {
			return "auth_refresh_failed", true
		}
	case "/api/v1/auth/reset-password":
		if statusCode == http.StatusUnauthorized {
			return "auth_reset_password_failed", true
		}
	case "/api/v1/auth/verify-email":
		if statusCode == http.StatusUnauthorized {
			return "auth_verify_email_failed", true
		}
	case "/api/v1/auth/register":
		if statusCode == http.StatusConflict {
			return "auth_register_duplicate_email", true
		}
	}

	return "", false
}

func writeSecurityEvent(logger *log.Logger, c *gin.Context, clientKey string, event string, statusCode int) {
	userAgent := strings.TrimSpace(c.Request.UserAgent())
	if len(userAgent) > 160 {
		userAgent = userAgent[:160]
	}

	logger.Printf(
		"security_event=%s method=%s path=%s ip=%s status=%d ua=%q",
		event,
		c.Request.Method,
		c.Request.URL.Path,
		clientKey,
		statusCode,
		userAgent,
	)
}

func requestClientKey(request *http.Request, trustForwardedHeaders bool, trustedProxyNets []*net.IPNet) string {
	if request == nil {
		return "unknown"
	}

	if trustForwardedHeaders && requestFromTrustedProxy(request, trustedProxyNets) {
		clientIP := forwardedForClientIP(request.Header.Get("X-Forwarded-For"))
		if clientIP != "" {
			return clientIP
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(request.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if strings.TrimSpace(request.RemoteAddr) != "" {
		return strings.TrimSpace(request.RemoteAddr)
	}

	return "unknown"
}

func isHTTPSRequest(request *http.Request, trustForwardedHeaders bool, trustedProxyNets []*net.IPNet) bool {
	if request == nil {
		return false
	}

	if request.TLS != nil {
		return true
	}

	if trustForwardedHeaders && requestFromTrustedProxy(request, trustedProxyNets) {
		if strings.EqualFold(firstHeaderToken(request.Header.Get("X-Forwarded-Proto")), "https") {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(request.Header.Get("X-Forwarded-Ssl")), "on") {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(request.Header.Get("X-Url-Scheme")), "https") {
			return true
		}
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

func parseTrustedProxyNetworks(values []string) []*net.IPNet {
	if len(values) == 0 {
		return nil
	}

	networks := make([]*net.IPNet, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}

		if strings.Contains(value, "/") {
			_, network, err := net.ParseCIDR(value)
			if err == nil && network != nil {
				networks = append(networks, network)
			}
			continue
		}

		ip := net.ParseIP(value)
		if ip == nil {
			continue
		}
		mask := net.CIDRMask(32, 32)
		if ip.To4() == nil {
			mask = net.CIDRMask(128, 128)
		}
		networks = append(networks, &net.IPNet{IP: ip, Mask: mask})
	}

	return networks
}

func requestFromTrustedProxy(request *http.Request, trustedProxyNets []*net.IPNet) bool {
	if request == nil || len(trustedProxyNets) == 0 {
		return false
	}

	host := strings.TrimSpace(request.RemoteAddr)
	if host == "" {
		return false
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil && parsedHost != "" {
		host = parsedHost
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	for _, network := range trustedProxyNets {
		if network != nil && network.Contains(ip) {
			return true
		}
	}

	return false
}

func forwardedForClientIP(headerValue string) string {
	first := firstHeaderToken(headerValue)
	if first == "" {
		return ""
	}

	ip := net.ParseIP(first)
	if ip == nil {
		return ""
	}

	return ip.String()
}

func isProductionEnvironment(env string) bool {
	normalized := strings.ToLower(strings.TrimSpace(env))
	return normalized == appEnvProduction || normalized == appEnvProd
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
