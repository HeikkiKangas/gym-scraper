package main

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/gocolly/colly"
)

type AdaptiveLimiter struct {
	mu               sync.Mutex
	delay            time.Duration
	minDelay         time.Duration
	maxDelay         time.Duration
	healthyResponses int
	nextRequest      time.Time
}

func NewAdaptiveLimiter(phaseCfg PhaseConfig, cfg ScraperConfig) *AdaptiveLimiter {
	return &AdaptiveLimiter{
		delay:    phaseCfg.Delay,
		minDelay: cfg.MinDelay,
		maxDelay: cfg.MaxDelay,
	}
}

func (l *AdaptiveLimiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	wait := time.Until(l.nextRequest)
	if wait < 0 {
		wait = 0
	}
	l.nextRequest = time.Now().Add(wait + l.delay)
	l.mu.Unlock()

	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (l *AdaptiveLimiter) Observe(statusCode int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err == nil && statusCode >= 200 && statusCode < 300 {
		l.healthyResponses++
		if l.healthyResponses >= 10 {
			l.delay = max(l.minDelay, l.delay-l.delay/10)
			l.healthyResponses = 0
		}
		return
	}

	l.healthyResponses = 0
	var netErr net.Error
	switch {
	case statusCode == 403 || statusCode == 429:
		l.delay = min(l.maxDelay, max(l.minDelay, l.delay*2))
	case statusCode >= 500 && statusCode <= 599:
		l.delay = min(l.maxDelay, max(l.minDelay, l.delay+l.delay/2))
	case errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()):
		l.delay = min(l.maxDelay, max(l.minDelay, l.delay+l.delay/2))
	}
}

func (l *AdaptiveLimiter) Delay() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.delay
}

func registerAdaptiveHooks(c *colly.Collector, limiter *AdaptiveLimiter) {
	c.OnResponse(func(r *colly.Response) {
		limiter.Observe(r.StatusCode, nil)
	})
	c.OnError(func(r *colly.Response, err error) {
		statusCode := 0
		if r != nil {
			statusCode = r.StatusCode
		}
		limiter.Observe(statusCode, err)
	})
}
