package main

import (
	"net/url"
	"strings"
)

const scraperBaseURL = "https://kuntosali.fi"

func NormalizeURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	base, err := url.Parse(scraperBaseURL)
	if err != nil {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if strings.Contains(raw, "://") && parsed.Scheme == "" {
		return "", false
	}
	absolute := base.ResolveReference(parsed)
	if absolute.Scheme != "http" && absolute.Scheme != "https" {
		return "", false
	}
	if absolute.Host == "" {
		return "", false
	}

	absolute.Fragment = ""
	absolute.Scheme = strings.ToLower(absolute.Scheme)
	absolute.Host = strings.ToLower(absolute.Host)
	absolute.Path = strings.TrimRight(absolute.Path, "/") + "/"
	return absolute.String(), true
}

func AddUniqueURL(seen map[string]struct{}, urls *[]string, raw string) bool {
	normalized, ok := NormalizeURL(raw)
	if !ok {
		return false
	}
	if _, exists := seen[normalized]; exists {
		return false
	}
	seen[normalized] = struct{}{}
	*urls = append(*urls, normalized)
	return true
}
