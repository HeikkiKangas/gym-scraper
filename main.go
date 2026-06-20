package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gocolly/colly"
)

const PARALLELISM = 4
const DELAY = 60
const RANDOM_DELAY = 120
const TIMEOUT = 120

func GetCities(metrics *PhaseMetrics) []string {
	cities := []string{}

	c := colly.NewCollector(colly.CacheDir("./cache"))
	c.SetRequestTimeout(TIMEOUT * time.Second)
	registerMetricsHooks(c, metrics)

	c.OnHTML("div.kaupunkilaatikko form#kaupunki-valinta select#kaupunki", func(e *colly.HTMLElement) {
		cities = append(cities, parseCitiesFromSelection(e.DOM)...)
	})

	c.OnError(func(r *colly.Response, e error) {
		fmt.Println("Request URL:", r.Request.URL, "\nError:", e)
	})

	c.Visit("https://kuntosali.fi")

	return cities
}

func GetGyms(cities []string, metrics *PhaseMetrics) []string {
	gyms := []string{}
	var mu sync.Mutex

	c := colly.NewCollector(colly.Async(true), colly.CacheDir("./cache"))
	c.Limit(&colly.LimitRule{
		Parallelism: PARALLELISM,
		Delay:       DELAY * time.Second,
		RandomDelay: RANDOM_DELAY * time.Second,
	})
	c.SetRequestTimeout(TIMEOUT * time.Second)
	registerMetricsHooks(c, metrics)

	c.OnHTML("div.salilistaus-simple a.salin-nimi-kaupunki[href]", func(e *colly.HTMLElement) {
		url, ok := parseGymURLFromElement(e)
		if !ok {
			return
		}
		mu.Lock()
		gyms = append(gyms, url)
		mu.Unlock()
	})

	c.OnError(func(r *colly.Response, e error) {
		fmt.Println("Request URL:", r.Request.URL, "\nError:", e)
	})

	for _, city := range cities {
		c.Visit(city)
	}

	c.Wait()

	return gyms
}

func GetEmails(gyms []string, metrics *PhaseMetrics) []string {
	emails := []string{}
	var mu sync.Mutex

	c := colly.NewCollector(colly.Async(true), colly.CacheDir("./cache"))
	c.Limit(&colly.LimitRule{
		Parallelism: PARALLELISM,
		Delay:       DELAY * time.Second,
		RandomDelay: RANDOM_DELAY * time.Second,
	})
	c.SetRequestTimeout(TIMEOUT * time.Second)
	registerMetricsHooks(c, metrics)

	c.OnHTML("div.sali-data div#salin-info p", func(e *colly.HTMLElement) {
		email, ok := parseEmailFromText(e.Text)
		if ok {
			mu.Lock()
			exists := slices.Contains(emails, email)
			if !exists {
				emails = append(emails, email)
			}
			mu.Unlock()
		}
	})

	c.OnError(func(r *colly.Response, e error) {
		fmt.Println("Request URL:", r.Request.URL, "\nError:", e)
	})

	for _, gym := range gyms {
		c.Visit(gym)
	}

	c.Wait()

	return emails
}

func main() {
	startTime := time.Now()
	metrics := NewScraperMetrics()

	fmt.Println("Collecting cities")
	metrics.Cities.Start()
	cities := GetCities(&metrics.Cities)
	metrics.Cities.Finish()
	fmt.Printf("Cities found: %d\n", len(cities))
	fmt.Printf("City phase elapsed: %.1f seconds\n", metrics.Cities.FinishedAt.Sub(metrics.Cities.StartedAt).Seconds())
	metrics.Cities.PrintSummary()

	fmt.Println("Collecting gyms")
	metrics.Gyms.Start()
	gyms := GetGyms(cities, &metrics.Gyms)
	metrics.Gyms.Finish()
	fmt.Printf("Gym URLs found: %d\n", len(gyms))
	fmt.Printf("Gym phase elapsed: %.1f seconds\n", metrics.Gyms.FinishedAt.Sub(metrics.Gyms.StartedAt).Seconds())
	metrics.Gyms.PrintSummary()

	fmt.Println("Collecting emails")
	metrics.Emails.Start()
	emails := GetEmails(gyms, &metrics.Emails)
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
