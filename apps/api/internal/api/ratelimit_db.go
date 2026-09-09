package api

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// dbRateLimiter persists counters in Postgres so limits survive restarts and apply across API replicas.
// Falls back to the in-memory limiter when the pool is nil or the DB call fails.
type dbRateLimiter struct {
	pool   *pgxpool.Pool
	memory *rateLimiter
	limit  int
	window time.Duration
	prefix string
}

func newDBRateLimiter(pool *pgxpool.Pool, prefix string, limit int, window time.Duration) *dbRateLimiter {
	return &dbRateLimiter{
		pool:   pool,
		memory: newRateLimiter(limit, window),
		limit:  limit,
		window: window,
		prefix: prefix,
	}
}

func (rl *dbRateLimiter) allow(ctx context.Context, key string) bool {
	if rl.pool == nil {
		return rl.memory.allow(key)
	}
	fullKey := rl.prefix + ":" + key
	cctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	windowLiteral := fmt.Sprintf("%d seconds", int(rl.window.Seconds()))
	var count int
	err := rl.pool.QueryRow(cctx, `
		INSERT INTO rate_limit_buckets (bucket_key, count, reset_at)
		VALUES ($1, 1, now() + $2::interval)
		ON CONFLICT (bucket_key) DO UPDATE SET
			count = CASE
				WHEN rate_limit_buckets.reset_at <= now() THEN 1
				ELSE rate_limit_buckets.count + 1
			END,
			reset_at = CASE
				WHEN rate_limit_buckets.reset_at <= now() THEN now() + $2::interval
				ELSE rate_limit_buckets.reset_at
			END
		RETURNING count`, fullKey, windowLiteral).Scan(&count)
	if err != nil {
		log.Printf("rate limit db fallback to memory: %v", err)
		return rl.memory.allow(key)
	}
	return count <= rl.limit
}

var loginLimiter *dbRateLimiter
var enrollLimiter *dbRateLimiter

func initRateLimiters(pool *pgxpool.Pool) {
	loginLimiter = newDBRateLimiter(pool, "login", 20, time.Minute)
	enrollLimiter = newDBRateLimiter(pool, "enroll", 30, time.Minute)
}
