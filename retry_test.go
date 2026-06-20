package main

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestShouldRetry(t *testing.T) {
	tests := []struct {
		name   string
		status int
		err    error
		want   bool
	}{
		{name: "rate limited", status: http.StatusTooManyRequests, want: true},
		{name: "server error", status: http.StatusServiceUnavailable, want: true},
		{name: "not found", status: http.StatusNotFound, want: false},
		{name: "timeout", err: timeoutError{}, want: true},
		{name: "parser failure", err: errors.New("invalid markup"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldRetry(tt.status, tt.err); got != tt.want {
				t.Fatalf("ShouldRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryDelayUsesExponentialBounds(t *testing.T) {
	base := 10 * time.Millisecond
	for attempt := 1; attempt <= 3; attempt++ {
		minimum := time.Duration(1<<(attempt-1)) * base
		maximum := minimum * 2
		got := RetryDelay(attempt, base)
		if got < minimum || got >= maximum {
			t.Fatalf("attempt %d delay %s outside [%s, %s)", attempt, got, minimum, maximum)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	if got, ok := ParseRetryAfter("7", now); !ok || got != 7*time.Second {
		t.Fatalf("seconds Retry-After = (%s, %v)", got, ok)
	}
	date := now.Add(12 * time.Second).Format(http.TimeFormat)
	if got, ok := ParseRetryAfter(date, now); !ok || got != 12*time.Second {
		t.Fatalf("date Retry-After = (%s, %v)", got, ok)
	}
}

func TestTimeoutIsRetriedAndRecorded(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			time.Sleep(40 * time.Millisecond)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultScraperConfig()
	phaseCfg := PhaseConfig{
		Parallelism: 1,
		Delay:       time.Millisecond,
		Timeout:     10 * time.Millisecond,
	}
	cfg.MinDelay = time.Millisecond
	cfg.MaxRetries = 1
	metrics := newPhaseMetrics("timeout-test")
	c := NewCollector(phaseCfg)
	registerMetricsHooks(c, metrics)
	limiter := NewAdaptiveLimiter(phaseCfg, cfg)
	registerAdaptiveHooks(c, limiter)
	registerRetryHook(c, phaseCfg, cfg, metrics, limiter)

	if err := VisitWithRetry(c, server.URL, metrics); err != nil {
		t.Fatalf("VisitWithRetry returned error: %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
	if metrics.Retries != 1 || metrics.Timeouts != 1 {
		t.Fatalf("retry metrics = retries:%d timeouts:%d, want 1 and 1", metrics.Retries, metrics.Timeouts)
	}
	if got := len(metrics.FailedURLsSnapshot()); got != 0 {
		t.Fatalf("failed URLs = %d, want 0", got)
	}
}

func TestRetryRecordsErrorStatusAndSuccessfulResponse(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultScraperConfig()
	phaseCfg := PhaseConfig{Parallelism: 1, Delay: time.Millisecond, Timeout: time.Second}
	cfg.MinDelay = time.Millisecond
	cfg.MaxRetries = 1
	metrics := newPhaseMetrics("status-test")
	c := NewCollector(phaseCfg)
	registerMetricsHooks(c, metrics)
	limiter := NewAdaptiveLimiter(phaseCfg, cfg)
	registerAdaptiveHooks(c, limiter)
	registerRetryHook(c, phaseCfg, cfg, metrics, limiter)

	if err := VisitWithRetry(c, server.URL, metrics); err != nil {
		t.Fatalf("VisitWithRetry returned error: %v", err)
	}
	if metrics.StatusCounts[http.StatusServiceUnavailable] != 1 || metrics.StatusCounts[http.StatusOK] != 1 {
		t.Fatalf("status counts = %#v, want one 503 and one 200", metrics.StatusCounts)
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}
