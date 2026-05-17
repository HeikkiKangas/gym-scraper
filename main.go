package main

import (
	"fmt"
	"net/mail"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
)

const PARALLELISM = 4
const DELAY = 60
const RANDOM_DELAY = 120
const TIMEOUT = 120

func GetCities() []string {
	cities := []string{}

	c := colly.NewCollector(colly.CacheDir("./cache"))
	c.SetRequestTimeout(TIMEOUT * time.Second)

	c.OnHTML("div.kaupunkilaatikko form#kaupunki-valinta select#kaupunki", func(e *colly.HTMLElement) {
		e.DOM.Children().Each(func(i int, s *goquery.Selection) {
			value, _ := s.Attr("value")
			if value != "-1" {
				cities = append(cities, fmt.Sprintf("https://kuntosali.fi/kaupungit/%v/", value))
			}
		})
	})

	c.OnError(func(r *colly.Response, e error) {
		fmt.Println("Request URL:", r.Request.URL, "\nError:", e)
	})

	c.Visit("https://kuntosali.fi")

	return cities
}

func GetGyms(cities []string) []string {
	gyms := []string{}
	var mu sync.Mutex

	c := colly.NewCollector(colly.Async(true), colly.CacheDir("./cache"))
	c.Limit(&colly.LimitRule{
		Parallelism: PARALLELISM,
		Delay:       DELAY * time.Second,
		RandomDelay: RANDOM_DELAY * time.Second,
	})
	c.SetRequestTimeout(TIMEOUT * time.Second)

	c.OnHTML("div.salilistaus-simple a.salin-nimi-kaupunki[href]", func(e *colly.HTMLElement) {
		url := e.Attr("href")
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

func GetEmails(gyms []string) []string {
	emails := []string{}
	var mu sync.Mutex

	c := colly.NewCollector(colly.Async(true), colly.CacheDir("./cache"))
	c.Limit(&colly.LimitRule{
		Parallelism: PARALLELISM,
		Delay:       DELAY * time.Second,
		RandomDelay: RANDOM_DELAY * time.Second,
	})
	c.SetRequestTimeout(TIMEOUT * time.Second)

	c.OnHTML("div.sali-data div#salin-info p", func(e *colly.HTMLElement) {
		text := strings.TrimSpace(e.Text)
		addr, err := mail.ParseAddress(text)
		if err == nil {
			text = addr.Address
		}

		if isValidEmail(text) {
			mu.Lock()
			exists := slices.Contains(emails, text)
			if !exists {
				emails = append(emails, text)
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

	fmt.Println("Collecting cities")
	cities := GetCities()

	fmt.Println("Collecting gyms")
	gyms := GetGyms(cities)

	fmt.Println("Collecting emails")
	emails := GetEmails(gyms)

	fmt.Println("Emails found: ", len(emails))
	fmt.Printf("Finished in %v seconds\n", time.Since(startTime).Seconds())

	if err := os.WriteFile("emails.txt", []byte(strings.Join(emails, "\n")), 0644); err != nil {
		fmt.Println("Failed to write emails.txt:", err)
		os.Exit(1)
	}

	if err := os.RemoveAll("./cache/"); err != nil {
		fmt.Println("Failed to remove cache directory:", err)
		os.Exit(1)
	}
}

func isValidEmail(s string) bool {
	if strings.Count(s, "@") != 1 {
		return false
	}

	addr, err := mail.ParseAddress(s)
	if err != nil {
		return false
	}

	return addr.Address == s
}
