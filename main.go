package main

import (
	"context"
	"fmt"
	"os"
	"strings"
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

func main() {
	startTime := time.Now()
	cfg, err := LoadScraperConfig()
	if err != nil {
		fmt.Println("Invalid scraper configuration:", err)
		os.Exit(2)
	}

	result := ScrapeEmails(context.Background(), cfg)
	metrics := result.Metrics

	fmt.Printf("Cities found: %d\n", result.CitiesFound)
	fmt.Printf("City phase elapsed: %.1f seconds\n", metrics.Cities.FinishedAt.Sub(metrics.Cities.StartedAt).Seconds())
	metrics.Cities.PrintSummary()
	fmt.Printf("Gym URLs found: %d\n", result.GymURLsFound)
	fmt.Printf("Gym phase elapsed: %.1f seconds\n", metrics.Gyms.FinishedAt.Sub(metrics.Gyms.StartedAt).Seconds())
	metrics.Gyms.PrintSummary()
	fmt.Printf("Emails found: %d\n", len(result.Emails))
	fmt.Printf("Email phase elapsed: %.1f seconds\n", metrics.Emails.FinishedAt.Sub(metrics.Emails.StartedAt).Seconds())
	metrics.Emails.PrintSummary()

	fmt.Printf("Total elapsed: %.1f seconds\n", time.Since(startTime).Seconds())

	if err := os.WriteFile("emails.txt", []byte(strings.Join(result.Emails, "\n")), 0644); err != nil {
		fmt.Println("Failed to write emails.txt:", err)
		os.Exit(1)
	}

	if err := os.RemoveAll("./cache/"); err != nil {
		fmt.Println("Failed to remove cache directory:", err)
		os.Exit(1)
	}
}
