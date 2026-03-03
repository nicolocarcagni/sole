package main

import (
	"bytes"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// IPRateLimiter mananges rate limiters for each IP
type IPRateLimiter struct {
	ips map[string]*rate.Limiter
	mu  sync.Mutex
	r   rate.Limit
	b   int
}

// NewIPRateLimiter creates a new limiter
func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	i := &IPRateLimiter{
		ips: make(map[string]*rate.Limiter),
		r:   r,
		b:   b,
	}

	// Clean up old entries periodically (Proof Of concept)
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			i.mu.Lock()
			// In production you'd track last access time and delete old ones
			// For now, simple clear to prevent memory leaks in long run
			i.ips = make(map[string]*rate.Limiter)
			i.mu.Unlock()
		}
	}()

	return i
}

// GetLimiter returns the limiter for an IP
func (i *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	i.mu.Lock()
	defer i.mu.Unlock()

	limiter, exists := i.ips[ip]
	if !exists {
		limiter = rate.NewLimiter(i.r, i.b)
		i.ips[ip] = limiter
	}

	return limiter
}

// RateLimitMiddleware creates a middleware for rate limiting
func RateLimitMiddleware(limiter *IPRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			// 1. Safe Header Parsing for IP (Handles proxies like Nginx)
			ip := ""
			forwarded := r.Header.Get("X-Forwarded-For")
			if forwarded != "" {
				parts := strings.Split(forwarded, ",")
				if len(parts) > 0 && parts[0] != "" {
					ip = strings.TrimSpace(parts[0])
				}
			}

			// Fallback to RemoteAddr if no proxy header exists
			if ip == "" {
				var err error
				ip, _, err = net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					ip = r.RemoteAddr
				}
			}

			// 2. Check Limit safely
			l := limiter.GetLimiter(ip)
			if !l.Allow() {
				http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
				return
			}

			// Security Header
			w.Header().Set("X-Content-Type-Options", "nosniff")

			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware handles Cross-Origin Resource Sharing & Panic Recovery & Safe Body reading
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// 1. Panic Recovery Middleware
		defer func() {
			if err := recover(); err != nil {
				log.Printf("⚠️  [REST API] Recovered from panic in handler: %v", err)
				http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
			}
		}()

		// 2. Safe Body Reading (Read and replace properly)
		if r.Body != nil {
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil {
				// Restore the body so the underlying handler can read it safely
				r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}
		}

		// 3. Safe Header Extraction
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		// Set CORS Headers
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Origin, Accept, Authorization, X-CSRF-Token")

		// Handle Preflight
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
