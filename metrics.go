package main

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gocolly/colly"
)

type PhaseMetrics struct {
	Name             string
	StartedAt        time.Time
	FinishedAt       time.Time
	RequestsStarted  int64
	RequestsFinished int64
	RequestsFailed   int64
	Retries          int64
	Timeouts         int64
	Throttles        int64
	StatusCounts     map[int]int64
	Latencies        []time.Duration
	mu               sync.Mutex
}

type ScraperMetrics struct {
	Cities PhaseMetrics
	Gyms   PhaseMetrics
	Emails PhaseMetrics
}

func NewScraperMetrics() ScraperMetrics {
	return ScraperMetrics{
		Cities: newPhaseMetrics("cities"),
		Gyms:   newPhaseMetrics("gyms"),
		Emails: newPhaseMetrics("emails"),
	}
}

func newPhaseMetrics(name string) PhaseMetrics {
	return PhaseMetrics{Name: name, StatusCounts: make(map[int]int64)}
}

func (m *PhaseMetrics) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StartedAt = time.Now()
}

func (m *PhaseMetrics) Finish() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FinishedAt = time.Now()
}

func (m *PhaseMetrics) RecordRequestStart() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RequestsStarted++
}

func (m *PhaseMetrics) RecordResponse(statusCode int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RequestsFinished++
	m.StatusCounts[statusCode]++
	m.Latencies = append(m.Latencies, duration)
	if statusCode == 403 || statusCode == 429 {
		m.Throttles++
	}
}

func (m *PhaseMetrics) RecordFailure(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RequestsFailed++
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		m.Timeouts++
	}
}

func (m *PhaseMetrics) RecordRetry() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Retries++
}

func registerMetricsHooks(c *colly.Collector, metrics *PhaseMetrics) {
	c.OnRequest(func(r *colly.Request) {
		metrics.RecordRequestStart()
		r.Ctx.Put("startedAt", time.Now())
	})
	c.OnResponse(func(r *colly.Response) {
		startedAt, ok := r.Ctx.GetAny("startedAt").(time.Time)
		if !ok {
			startedAt = time.Now()
		}
		metrics.RecordResponse(r.StatusCode, time.Since(startedAt))
	})
	c.OnError(func(_ *colly.Response, err error) {
		metrics.RecordFailure(err)
	})
}

func (m *PhaseMetrics) PrintSummary() {
	m.mu.Lock()
	name := m.Name
	startedAt := m.StartedAt
	finishedAt := m.FinishedAt
	finished := m.RequestsFinished
	failed := m.RequestsFailed
	retries := m.Retries
	timeouts := m.Timeouts
	throttles := m.Throttles
	statuses := make(map[int]int64, len(m.StatusCounts))
	for status, count := range m.StatusCounts {
		statuses[status] = count
	}
	latencies := append([]time.Duration(nil), m.Latencies...)
	m.mu.Unlock()

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	var total time.Duration
	for _, latency := range latencies {
		total += latency
	}
	var average, p95 time.Duration
	if len(latencies) > 0 {
		average = total / time.Duration(len(latencies))
		p95 = latencies[(len(latencies)*95-1)/100]
	}

	statusCodes := make([]int, 0, len(statuses))
	for status := range statuses {
		statusCodes = append(statusCodes, status)
	}
	sort.Ints(statusCodes)
	statusParts := make([]string, 0, len(statusCodes))
	for _, status := range statusCodes {
		statusParts = append(statusParts, fmt.Sprintf("%d=%d", status, statuses[status]))
	}
	statusSummary := "none"
	if len(statusParts) > 0 {
		statusSummary = strings.Join(statusParts, ", ")
	}

	fmt.Printf("Phase: %s\n", name)
	fmt.Printf("Requests: %d completed, %d failed, %d retried\n", finished, failed, retries)
	fmt.Printf("Status: %s\n", statusSummary)
	fmt.Printf("Latency: avg=%s, p95=%s\n", average.Round(time.Millisecond), p95.Round(time.Millisecond))
	fmt.Printf("Timeouts: %d, throttles: %d\n", timeouts, throttles)
	fmt.Printf("Elapsed: %.1fs\n", finishedAt.Sub(startedAt).Seconds())
}
