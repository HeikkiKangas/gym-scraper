package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
)

func CollectCities() []string {
	collector := colly.NewCollector()

	cities := []string{}
	citiesCollected := false

	collector.OnHTML("#kaupunki", func(e *colly.HTMLElement) {
		if !citiesCollected {
			e.DOM.Children().Each(func(i int, s *goquery.Selection) {
				value, _ := s.Attr("value")
				if value != "-1" {
					cities = append(cities, value)
				}
			})
			citiesCollected = true
		}
	})

	collector.Visit("https://kuntosali.fi")
	return cities
}

func CollectGymUrls(city string) []string {
	collector := colly.NewCollector()

	gymUrls := []string{}

	collector.OnHTML(".salilistaus-simple", func(e *colly.HTMLElement) {
		url, _ := e.DOM.Children().Children().Attr("href")
		gymUrls = append(gymUrls, url)
	})

	collector.Visit(fmt.Sprintf("https://kuntosali.fi/kaupungit/%v/", city))
	return gymUrls
}

func CollectGymEmail(gymUrl string) string {
	collector := colly.NewCollector()
	email := ""
	collector.OnHTML("#salin-info", func(e *colly.HTMLElement) {
		text := e.DOM.Children().Text()
		if strings.Contains(text, "@") {
			email = text
		}
	})

	collector.Visit(gymUrl)
	return email
}

func main() {
	fmt.Println("Getting cities with gyms")
	cities := CollectCities()
	gymUrls := []string{}
	emails := []string{}

	fmt.Printf("Found %v cities\n", len(cities))

	for i, city := range cities {
		fmt.Printf("Getting gym urls, city %v/%v\n", i+1, len(cities))
		gymUrls = append(gymUrls, CollectGymUrls(city)...)
	}

	fmt.Println(gymUrls)

	for i, gymUrl := range gymUrls {
		fmt.Printf("Getting gym email, gym %v/%v\n", i+1, len(gymUrls))
		email := CollectGymEmail(gymUrl)
		if email != "" && !slices.Contains(emails, email) {
			emails = append(emails, email)
		}
	}

	fmt.Println("Emails found: ", len(emails))
	os.WriteFile("emails.txt", []byte(strings.Join(emails, "\n")), 0644)
}
