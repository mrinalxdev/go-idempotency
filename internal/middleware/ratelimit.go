package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	client *redis.Client
	limit int
	window time.Duration
}

func NewRateLimiter(client *redis.Client, limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		client: client,
		limit:  limit,
		window: window,
	}
}

func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
		ip := r.RemoteAddr
		key := fmt.Sprintf("ratelimit:%s", ip)
		ctx := context.Background()

		pipe := rl.client.Pipeline()
		incr := pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, rl.window)
		_, err := pipe.Exec(ctx)
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		count := incr.Val()
		if count > int64(rl.limit) {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", rl.window.Seconds()))
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}