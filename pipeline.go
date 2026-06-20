package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/gocolly/colly"
)

type GymResult struct {
	URL    string
	Email  string
	Err    error
	Status int
}

type ScrapeResult struct {
	Emails       []string
	FailedURLs   []string
	GymURLsFound int
	CitiesFound  int
	Metrics      ScraperMetrics
}

func ScrapeEmails(ctx context.Context, cfg ScraperConfig) ScrapeResult {
	metrics := NewScraperMetrics()

	fmt.Println("Collecting cities")
	metrics.Cities.Start()
	cities := GetCities(cfg, metrics.Cities)
	metrics.Cities.Finish()

	fmt.Println("Collecting gyms")
	fmt.Println("Collecting emails")
	metrics.Gyms.Start()
	metrics.Emails.Start()

	gymURLCh := make(chan string, 100)
	emailCh := make(chan string, 100)
	var gymCount atomic.Int64

	go func() {
		ProduceGymURLs(ctx, cities, gymURLCh, cfg, metrics.Gyms, &gymCount)
		metrics.Gyms.Finish()
		close(gymURLCh)
	}()
	go func() {
		ConsumeGymURLs(ctx, gymURLCh, emailCh, cfg, metrics.Emails)
		metrics.Emails.Finish()
		close(emailCh)
	}()

	emails := CollectEmails(emailCh)
	failedURLs := append(metrics.Cities.FailedURLsSnapshot(), metrics.Gyms.FailedURLsSnapshot()...)
	failedURLs = append(failedURLs, metrics.Emails.FailedURLsSnapshot()...)
	return ScrapeResult{
		Emails:       emails,
		FailedURLs:   failedURLs,
		GymURLsFound: int(gymCount.Load()),
		CitiesFound:  len(cities),
		Metrics:      metrics,
	}
}

func ProduceGymURLs(
	ctx context.Context,
	cities []string,
	out chan<- string,
	cfg ScraperConfig,
	metrics *PhaseMetrics,
	gymCount *atomic.Int64,
) {
	jobs := make(chan string)
	seen := make(map[string]struct{})
	var seenMu sync.Mutex
	limiter := NewAdaptiveLimiter(cfg)
	var workers sync.WaitGroup

	for range cfg.GymParallelism {
		workers.Add(1)
		go func() {
			defer workers.Done()
			c := newPipelineCollector(cfg, limiter, metrics)
			c.OnHTML("div.salilistaus-simple a.salin-nimi-kaupunki[href]", func(e *colly.HTMLElement) {
				normalized, ok := NormalizeURL(e.Attr("href"))
				if !ok {
					return
				}

				seenMu.Lock()
				_, exists := seen[normalized]
				if !exists {
					seen[normalized] = struct{}{}
					gymCount.Add(1)
				}
				seenMu.Unlock()
				if exists {
					return
				}

				select {
				case out <- normalized:
				case <-ctx.Done():
				}
			})
			c.OnError(func(r *colly.Response, err error) {
				fmt.Printf("city page failed url=%s error=%v\n", responseURL(r), err)
			})

			for {
				select {
				case <-ctx.Done():
					return
				case city, ok := <-jobs:
					if !ok {
						return
					}
					if err := VisitWithRetry(c, city, cfg, metrics); err != nil {
						metrics.RecordFailedURL(city)
						fmt.Printf("Failed to schedule city URL %s: %v\n", city, err)
					}
				}
			}
		}()
	}

	for _, city := range cities {
		select {
		case jobs <- city:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		}
	}
	close(jobs)
	workers.Wait()
}

func ConsumeGymURLs(ctx context.Context, in <-chan string, out chan<- string, cfg ScraperConfig, metrics *PhaseMetrics) {
	limiter := NewAdaptiveLimiter(cfg)
	var workers sync.WaitGroup
	for range cfg.EmailParallelism {
		workers.Add(1)
		go func() {
			defer workers.Done()
			c := newPipelineCollector(cfg, limiter, metrics)
			c.OnHTML("div.sali-data div#salin-info p", func(e *colly.HTMLElement) {
				email, ok := parseEmailFromText(e.Text)
				if !ok {
					return
				}
				select {
				case out <- email:
				case <-ctx.Done():
				}
			})
			c.OnError(func(r *colly.Response, err error) {
				fmt.Printf("gym page failed url=%s error=%v\n", responseURL(r), err)
			})

			for {
				select {
				case <-ctx.Done():
					return
				case gym, ok := <-in:
					if !ok {
						return
					}
					if err := VisitWithRetry(c, gym, cfg, metrics); err != nil {
						metrics.RecordFailedURL(gym)
						fmt.Printf("Failed to schedule gym URL %s: %v\n", gym, err)
					}
				}
			}
		}()
	}
	workers.Wait()
}

func CollectEmails(in <-chan string) []string {
	emails := []string{}
	seen := make(map[string]struct{})
	for email := range in {
		if _, exists := seen[email]; exists {
			continue
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
	}
	return emails
}

func newPipelineCollector(cfg ScraperConfig, limiter *AdaptiveLimiter, metrics *PhaseMetrics) *colly.Collector {
	c := colly.NewCollector(colly.CacheDir("./cache"))
	c.Limit(&colly.LimitRule{DomainGlob: "*", Parallelism: 1, RandomDelay: cfg.RandomDelay})
	c.SetRequestTimeout(cfg.Timeout)
	registerMetricsHooks(c, metrics)
	registerAdaptiveHooks(c, limiter)
	registerRetryHook(c, cfg, metrics)
	return c
}

func responseURL(r *colly.Response) string {
	if r == nil || r.Request == nil || r.Request.URL == nil {
		return "unknown"
	}
	return r.Request.URL.String()
}
