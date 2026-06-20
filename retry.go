package main

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly"
)

func ShouldRetry(statusCode int, err error) bool {
	switch statusCode {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}

	message := strings.ToLower(fmt.Sprint(err))
	return strings.Contains(message, "connection reset") ||
		strings.Contains(message, "temporary failure in name resolution") ||
		strings.Contains(message, "server misbehaving")
}

func RetryDelay(attempt int, baseDelay time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if baseDelay <= 0 {
		baseDelay = time.Second
	}
	multiplier := time.Duration(1 << (attempt - 1))
	backoff := multiplier * baseDelay
	return backoff + time.Duration(rand.Int64N(int64(backoff)))
}

func ParseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	delay := when.Sub(now)
	if delay < 0 {
		delay = 0
	}
	return delay, true
}

func VisitWithRetry(c *colly.Collector, rawURL string, cfg ScraperConfig, metrics *PhaseMetrics) error {
	ctx := colly.NewContext()
	ctx.Put("attempt", 0)
	return c.Request(http.MethodGet, rawURL, nil, ctx, nil)
}

func registerRetryHook(c *colly.Collector, cfg ScraperConfig, metrics *PhaseMetrics) {
	c.OnError(func(r *colly.Response, err error) {
		if r == nil || r.Request == nil || !ShouldRetry(r.StatusCode, err) {
			if r != nil && r.Request != nil {
				metrics.RecordFailedURL(r.Request.URL.String())
			}
			return
		}

		attempt, _ := r.Ctx.GetAny("attempt").(int)
		if attempt >= cfg.MaxRetries {
			metrics.RecordFailedURL(r.Request.URL.String())
			fmt.Printf("request failed phase=%s url=%s status=%d attempts=%d error=%v\n",
				metrics.Name, r.Request.URL, r.StatusCode, attempt+1, err)
			return
		}

		nextAttempt := attempt + 1
		delay := RetryDelay(nextAttempt, cfg.InitialDelay)
		if r.StatusCode == http.StatusTooManyRequests {
			if retryAfter, ok := ParseRetryAfter(r.Headers.Get("Retry-After"), time.Now()); ok {
				delay = retryAfter
			}
		}

		metrics.RecordRetry()
		r.Ctx.Put("attempt", nextAttempt)
		fmt.Printf("retry phase=%s url=%s status=%d attempt=%d retry_in=%s\n",
			metrics.Name, r.Request.URL, r.StatusCode, nextAttempt, delay.Round(time.Millisecond))
		time.Sleep(delay)
		if retryErr := r.Request.Retry(); retryErr != nil {
			metrics.RecordFailedURL(r.Request.URL.String())
			fmt.Printf("retry scheduling failed phase=%s url=%s error=%v\n", metrics.Name, r.Request.URL, retryErr)
		}
	})
}
