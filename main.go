package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
)

const PARALLELISM = 4
const DELAY = 30
const RANDOM_DELAY = 60
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
  
  c := colly.NewCollector(colly.Async(true), colly.CacheDir("./cache"))
  c.Limit(&colly.LimitRule{Parallelism: PARALLELISM, Delay: DELAY * time.Second, RandomDelay: RANDOM_DELAY * time.Second})
  c.SetRequestTimeout(TIMEOUT * time.Second)

  c.OnHTML("div.salilistaus-simple a.salin-nimi-kaupunki[href]", func(e *colly.HTMLElement) {
		url := e.Attr("href")
		gyms = append(gyms, url)
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
  
  c := colly.NewCollector(colly.Async(true), colly.CacheDir("./cache"))
  c.Limit(&colly.LimitRule{Parallelism: PARALLELISM, Delay: DELAY * time.Second, RandomDelay: RANDOM_DELAY * time.Second})
  c.SetRequestTimeout(TIMEOUT * time.Second)

  c.OnHTML("div.sali-data div#salin-info p", func(e *colly.HTMLElement) {
		text := e.Text
		if strings.Contains(text, "@") && !slices.Contains(emails, text) {
			emails = append(emails, strings.TrimSpace(text))
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
  
	os.WriteFile("emails.txt", []byte(strings.Join(emails, "\n")), 0644)

	os.RemoveAll("./cache/")
}
