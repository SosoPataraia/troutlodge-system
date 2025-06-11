package middleware

import (
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

func RateLimit(rdb *redis.Client, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			ip := r.RemoteAddr // Simple IP-based rate limiting

			key := "rate_limit:" + ip

			current, err := rdb.Incr(ctx, key).Result()
			if err != nil {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			if current == 1 {
				rdb.Expire(ctx, key, window)
			}

			if current > int64(limit) {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
