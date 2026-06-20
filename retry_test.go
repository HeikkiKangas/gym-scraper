package main

import (
	"errors"
	"net"
	"net/http"
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

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}
