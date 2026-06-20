package main

import (
	"fmt"
	"net/mail"
	"slices"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/gocolly/colly"
)

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
	seen := make(map[string]struct{})
	doc.Find("div.salilistaus-simple a.salin-nimi-kaupunki[href]").Each(func(i int, s *goquery.Selection) {
		AddUniqueURL(seen, &urls, s.AttrOr("href", ""))
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
