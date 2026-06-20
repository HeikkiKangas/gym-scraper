package main

import (
	"reflect"
	"testing"
)

func TestParseCitiesFromHTML(t *testing.T) {
	html := `
<div class="kaupunkilaatikko">
  <form id="kaupunki-valinta">
    <select id="kaupunki">
      <option value="-1">Choose</option>
      <option value="helsinki">Helsinki</option>
      <option value="">Empty</option>
      <option value="tampere">Tampere</option>
    </select>
  </form>
</div>`

	got, err := parseCitiesFromHTML(html)
	if err != nil {
		t.Fatalf("parseCitiesFromHTML returned error: %v", err)
	}

	want := []string{
		"https://kuntosali.fi/kaupungit/helsinki/",
		"https://kuntosali.fi/kaupungit/tampere/",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected cities\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestParseGymURLsFromHTML(t *testing.T) {
	html := `
<div class="salilistaus-simple">
  <a class="salin-nimi-kaupunki" href="https://example.com/gym-1">Gym 1</a>
  <a class="salin-nimi-kaupunki" href="  https://example.com/gym-2  ">Gym 2</a>
	<a class="salin-nimi-kaupunki" href="https://example.com/gym-1/#details">Duplicate</a>
  <a class="salin-nimi-kaupunki">No href</a>
</div>`

	got, err := parseGymURLsFromHTML(html)
	if err != nil {
		t.Fatalf("parseGymURLsFromHTML returned error: %v", err)
	}

	want := []string{
		"https://example.com/gym-1/",
		"https://example.com/gym-2/",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected gym urls\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestParseEmailsFromHTML(t *testing.T) {
	html := `
<div class="sali-data">
  <div id="salin-info">
    <p>info@example.com</p>
    <p>Contact us</p>
    <p>Gym Team &lt;sales@example.com&gt;</p>
    <p>sales@example.com</p>
  </div>
</div>`

	got, err := parseEmailsFromHTML(html)
	if err != nil {
		t.Fatalf("parseEmailsFromHTML returned error: %v", err)
	}

	want := []string{
		"info@example.com",
		"sales@example.com",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected emails\ngot:  %#v\nwant: %#v", got, want)
	}
}

func TestParseEmailFromText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "plain", in: "hello@example.com", want: "hello@example.com", ok: true},
		{name: "name and address", in: "Team <team@example.com>", want: "team@example.com", ok: true},
		{name: "invalid", in: "hello at example.com", want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseEmailFromText(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("unexpected parse result got=(%q,%v) want=(%q,%v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}
