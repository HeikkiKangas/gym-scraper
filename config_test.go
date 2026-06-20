package main

import (
	"testing"
	"time"
)

func TestLoadScraperConfigFromEnvironment(t *testing.T) {
	t.Setenv("SCRAPER_CITY_PARALLELISM", "3")
	t.Setenv("SCRAPER_GYM_PARALLELISM", "8")
	t.Setenv("SCRAPER_EMAIL_PARALLELISM", "9")
	t.Setenv("SCRAPER_INITIAL_DELAY_SECONDS", "4")
	t.Setenv("SCRAPER_RANDOM_DELAY_SECONDS", "6")
	t.Setenv("SCRAPER_TIMEOUT_SECONDS", "20")
	t.Setenv("SCRAPER_MAX_RETRIES", "5")

	cfg, err := LoadScraperConfig()
	if err != nil {
		t.Fatalf("LoadScraperConfig returned error: %v", err)
	}
	if cfg.CityParallelism != 3 || cfg.GymParallelism != 8 || cfg.EmailParallelism != 9 {
		t.Fatalf("unexpected parallelism configuration: %#v", cfg)
	}
	if cfg.InitialDelay != 4*time.Second || cfg.RandomDelay != 6*time.Second || cfg.Timeout != 20*time.Second {
		t.Fatalf("unexpected duration configuration: %#v", cfg)
	}
	if cfg.MaxRetries != 5 {
		t.Fatalf("MaxRetries = %d, want 5", cfg.MaxRetries)
	}
}

func TestLoadScraperConfigRejectsInvalidValues(t *testing.T) {
	t.Setenv("SCRAPER_GYM_PARALLELISM", "0")
	if _, err := LoadScraperConfig(); err == nil {
		t.Fatal("LoadScraperConfig accepted zero parallelism")
	}
}

func TestAdaptiveLimiterRespondsToServerHealth(t *testing.T) {
	cfg := DefaultScraperConfig()
	limiter := NewAdaptiveLimiter(cfg)

	limiter.Observe(429, nil)
	if got := limiter.Delay(); got != 4*time.Second {
		t.Fatalf("delay after throttle = %s, want 4s", got)
	}

	for range 10 {
		limiter.Observe(200, nil)
	}
	if got := limiter.Delay(); got >= 4*time.Second {
		t.Fatalf("delay did not decrease after healthy responses: %s", got)
	}
}
