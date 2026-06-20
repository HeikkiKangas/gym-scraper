package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestPipelineDeduplicatesGymURLsAndEmails(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/city/one/", "/city/two/":
			fmt.Fprintf(w, `<div class="salilistaus-simple">
<a class="salin-nimi-kaupunki" href="%s/gym/a">Gym</a>
<a class="salin-nimi-kaupunki" href="%s/gym/a/#details">Duplicate</a>
</div>`, server.URL, server.URL)
		case "/gym/a/":
			fmt.Fprint(w, `<div class="sali-data"><div id="salin-info">
<p>info@example.com</p><p>info@example.com</p>
</div></div>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := DefaultScraperConfig()
	cfg.Gyms.Parallelism = 2
	cfg.Emails.Parallelism = 2
	cfg.Gyms.Delay = 0
	cfg.Emails.Delay = 0
	cfg.MinDelay = 0
	cfg.Gyms.RandomDelay = 0
	cfg.Emails.RandomDelay = 0
	cfg.Gyms.Timeout = 2 * time.Second
	cfg.Emails.Timeout = 2 * time.Second
	cfg.MaxRetries = 0
	metrics := NewScraperMetrics()
	gymCh := make(chan string, 2)
	emailCh := make(chan string, 2)
	var gymCount atomic.Int64

	go func() {
		ProduceGymURLs(context.Background(), []string{server.URL + "/city/one/", server.URL + "/city/two/"}, gymCh, cfg, metrics.Gyms, &gymCount)
		close(gymCh)
	}()
	go func() {
		ConsumeGymURLs(context.Background(), gymCh, emailCh, cfg, metrics.Emails)
		close(emailCh)
	}()

	got := CollectEmails(emailCh)
	if want := []string{"info@example.com"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("emails = %#v, want %#v", got, want)
	}
	if got := gymCount.Load(); got != 1 {
		t.Fatalf("gym URL count = %d, want 1", got)
	}
}

func TestCollectEmailsPreservesFirstSeenOrder(t *testing.T) {
	in := make(chan string, 4)
	for _, email := range []string{"a@example.com", "b@example.com", "a@example.com"} {
		in <- email
	}
	close(in)

	got := CollectEmails(in)
	want := []string{"a@example.com", "b@example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("emails = %#v, want %#v", got, want)
	}
}
