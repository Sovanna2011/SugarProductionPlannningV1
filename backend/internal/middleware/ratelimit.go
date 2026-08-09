package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter is a fixed-window counter used to throttle the login endpoint
// per IP and per user name (§C4).
//
// It is intentionally in-process: with one instance it is sufficient, and
// behind several instances the ingress or a shared store takes over. Being
// explicit about that is better than pretending an in-memory limiter is a
// distributed one.
type RateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	buckets map[string]*bucket
}

type bucket struct {
	count    int
	resetsAt time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if limit <= 0 {
		limit = 10
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{limit: limit, window: window, buckets: make(map[string]*bucket)}
}

// Allow reports whether one more attempt fits in the current window.
func (r *RateLimiter) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	entry, ok := r.buckets[key]
	if !ok || now.After(entry.resetsAt) {
		r.buckets[key] = &bucket{count: 1, resetsAt: now.Add(r.window)}
		r.sweepLocked(now)
		return true
	}
	if entry.count >= r.limit {
		return false
	}
	entry.count++
	return true
}

func (r *RateLimiter) sweepLocked(now time.Time) {
	for key, entry := range r.buckets {
		if now.After(entry.resetsAt) {
			delete(r.buckets, key)
		}
	}
}

// LoginRateLimit throttles by client IP and, when the body carries one, by
// user name — so neither a single address nor a single account can be used for
// an unbounded number of guesses.
func LoginRateLimit(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !limiter.Allow("ip:" + c.ClientIP()) {
			tooManyRequests(c)
			return
		}
		if username := c.GetHeader("X-Username"); username != "" {
			if !limiter.Allow("user:" + username) {
				tooManyRequests(c)
				return
			}
		}
		c.Next()
	}
}

// AllowLogin is called by the auth controller once the user name is known from
// the parsed body.
func (r *RateLimiter) AllowLogin(username string) bool {
	return r.Allow("user:" + username)
}

func tooManyRequests(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"success": false,
		"error": gin.H{
			"code":    "E-AUTH-429",
			"message": "Too many attempts, please wait before trying again",
		},
		"requestId": c.GetString(CtxRequestID),
	})
}
