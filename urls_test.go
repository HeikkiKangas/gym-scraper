package main

import (
	"reflect"
	"testing"
)

func TestNormalizeAndDeduplicateURLs(t *testing.T) {
	inputs := []string{
		"https://kuntosali.fi/salit/testi",
		"https://kuntosali.fi/salit/testi/",
		" https://KUNTOSALI.FI/salit/testi/#section ",
		"/salit/testi/",
	}

	seen := make(map[string]struct{})
	var got []string
	for _, input := range inputs {
		AddUniqueURL(seen, &got, input)
	}
	want := []string{"https://kuntosali.fi/salit/testi/"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected normalized URLs\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestNormalizeURLRejectsInvalidValues(t *testing.T) {
	for _, input := range []string{"", "   ", "://bad", "mailto:test@example.com"} {
		if got, ok := NormalizeURL(input); ok {
			t.Fatalf("NormalizeURL(%q) = %q, true", input, got)
		}
	}
}
