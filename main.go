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
		cities = append(cities, parseCitiesFromSelection(e.DOM)...)
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

func parseCitiesFromSelection(selection *goquery.Selection) []string {
	cities := []string{}
	selection.Children().Each(func(i int, s *goquery.Selection) {
		value, ok := s.Attr("value")
		if !ok || value == "-1" || strings.TrimSpace(value) == "" {
			return
		}
		cities = append(cities, fmt.Sprintf("https://kuntosali.fi/kaupungit/%v/", value))
	})
	return cities
}

func parseGymURLFromElement(e *colly.HTMLElement) (string, bool) {
	url := strings.TrimSpace(e.Attr("href"))
	return url, url != ""
}

func parseEmailFromText(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	addr, err := mail.ParseAddress(trimmed)
	if err == nil {
		trimmed = addr.Address
	}

	if !isValidEmail(trimmed) {
		return "", false
	}

	return trimmed, true
}

func parseCitiesFromHTML(html string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}
	selection := doc.Find("div.kaupunkilaatikko form#kaupunki-valinta select#kaupunki")
	return parseCitiesFromSelection(selection), nil
}

func parseGymURLsFromHTML(html string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	urls := []string{}
	doc.Find("div.salilistaus-simple a.salin-nimi-kaupunki[href]").Each(func(i int, s *goquery.Selection) {
		url := strings.TrimSpace(s.AttrOr("href", ""))
		if url != "" {
			urls = append(urls, url)
		}
	})

	return urls, nil
}

func parseEmailsFromHTML(html string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	emails := []string{}
	doc.Find("div.sali-data div#salin-info p").Each(func(i int, s *goquery.Selection) {
		email, ok := parseEmailFromText(s.Text())
		if ok && !slices.Contains(emails, email) {
			emails = append(emails, email)
		}
	})

	return emails, nil
}
