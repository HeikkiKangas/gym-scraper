package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gocolly/colly"
)

func GetCities(cfg ScraperConfig, metrics *PhaseMetrics) []string {
	cities := []string{}

	c := colly.NewCollector(colly.CacheDir("./cache"))
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: cfg.CityParallelism,
		RandomDelay: cfg.RandomDelay,
	})
	c.SetRequestTimeout(cfg.Timeout)
	registerMetricsHooks(c, metrics)
	registerAdaptiveHooks(c, NewAdaptiveLimiter(cfg))
	registerRetryHook(c, cfg, metrics)

	c.OnHTML("div.kaupunkilaatikko form#kaupunki-valinta select#kaupunki", func(e *colly.HTMLElement) {
		cities = append(cities, parseCitiesFromSelection(e.DOM)...)
	})

	c.OnError(func(r *colly.Response, e error) {
		fmt.Println("Request URL:", r.Request.URL, "\nError:", e)
	})

	if err := VisitWithRetry(c, "https://kuntosali.fi", cfg, metrics); err != nil {
		metrics.RecordFailedURL("https://kuntosali.fi")
		fmt.Println("Failed to schedule city collection:", err)
	}

	return cities
}

func GetGyms(cities []string, cfg ScraperConfig, metrics *PhaseMetrics) []string {
	gyms := []string{}
	seenGyms := make(map[string]struct{})
	var mu sync.Mutex

	c := colly.NewCollector(colly.Async(true), colly.CacheDir("./cache"))
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: cfg.GymParallelism,
		RandomDelay: cfg.RandomDelay,
	})
	c.SetRequestTimeout(cfg.Timeout)
	registerMetricsHooks(c, metrics)
	registerAdaptiveHooks(c, NewAdaptiveLimiter(cfg))
	registerRetryHook(c, cfg, metrics)

	c.OnHTML("div.salilistaus-simple a.salin-nimi-kaupunki[href]", func(e *colly.HTMLElement) {
		url, ok := parseGymURLFromElement(e)
		if !ok {
			return
		}
		mu.Lock()
		AddUniqueURL(seenGyms, &gyms, url)
		mu.Unlock()
	})

	c.OnError(func(r *colly.Response, e error) {
		fmt.Println("Request URL:", r.Request.URL, "\nError:", e)
	})

	for _, city := range cities {
		if err := VisitWithRetry(c, city, cfg, metrics); err != nil {
			metrics.RecordFailedURL(city)
			fmt.Printf("Failed to schedule city URL %s: %v\n", city, err)
		}
	}

	c.Wait()

	return gyms
}

func GetEmails(gyms []string, cfg ScraperConfig, metrics *PhaseMetrics) []string {
	emails := []string{}
	seenEmails := make(map[string]struct{})
	var mu sync.Mutex

	c := colly.NewCollector(colly.Async(true), colly.CacheDir("./cache"))
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: cfg.EmailParallelism,
		RandomDelay: cfg.RandomDelay,
	})
	c.SetRequestTimeout(cfg.Timeout)
	registerMetricsHooks(c, metrics)
	registerAdaptiveHooks(c, NewAdaptiveLimiter(cfg))
	registerRetryHook(c, cfg, metrics)

	c.OnHTML("div.sali-data div#salin-info p", func(e *colly.HTMLElement) {
		email, ok := parseEmailFromText(e.Text)
		if ok {
			mu.Lock()
			if _, exists := seenEmails[email]; !exists {
				seenEmails[email] = struct{}{}
				emails = append(emails, email)
			}
			mu.Unlock()
		}
	})

	c.OnError(func(r *colly.Response, e error) {
		fmt.Println("Request URL:", r.Request.URL, "\nError:", e)
	})

	for _, gym := range gyms {
		if err := VisitWithRetry(c, gym, cfg, metrics); err != nil {
			metrics.RecordFailedURL(gym)
			fmt.Printf("Failed to schedule gym URL %s: %v\n", gym, err)
		}
	}

	c.Wait()

	return emails
}

func main() {
	startTime := time.Now()
	metrics := NewScraperMetrics()
	cfg, err := LoadScraperConfig()
	if err != nil {
		fmt.Println("Invalid scraper configuration:", err)
		os.Exit(2)
	}

	fmt.Println("Collecting cities")
	metrics.Cities.Start()
	cities := GetCities(cfg, &metrics.Cities)
	metrics.Cities.Finish()
	fmt.Printf("Cities found: %d\n", len(cities))
	fmt.Printf("City phase elapsed: %.1f seconds\n", metrics.Cities.FinishedAt.Sub(metrics.Cities.StartedAt).Seconds())
	metrics.Cities.PrintSummary()

	fmt.Println("Collecting gyms")
	metrics.Gyms.Start()
	gyms := GetGyms(cities, cfg, &metrics.Gyms)
	metrics.Gyms.Finish()
	fmt.Printf("Gym URLs found: %d\n", len(gyms))
	fmt.Printf("Gym phase elapsed: %.1f seconds\n", metrics.Gyms.FinishedAt.Sub(metrics.Gyms.StartedAt).Seconds())
	metrics.Gyms.PrintSummary()

	fmt.Println("Collecting emails")
	metrics.Emails.Start()
	emails := GetEmails(gyms, cfg, &metrics.Emails)
	metrics.Emails.Finish()
	fmt.Printf("Emails found: %d\n", len(emails))
	fmt.Printf("Email phase elapsed: %.1f seconds\n", metrics.Emails.FinishedAt.Sub(metrics.Emails.StartedAt).Seconds())
	metrics.Emails.PrintSummary()

	fmt.Printf("Total elapsed: %.1f seconds\n", time.Since(startTime).Seconds())

	if err := os.WriteFile("emails.txt", []byte(strings.Join(emails, "\n")), 0644); err != nil {
		fmt.Println("Failed to write emails.txt:", err)
		os.Exit(1)
	}

	if err := os.RemoveAll("./cache/"); err != nil {
		fmt.Println("Failed to remove cache directory:", err)
		os.Exit(1)
	}
}
