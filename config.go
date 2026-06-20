package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type PhaseConfig struct {
	Parallelism int
	Delay       time.Duration
	RandomDelay time.Duration
	Timeout     time.Duration
}

type ScraperConfig struct {
	Cities PhaseConfig
	Gyms   PhaseConfig
	Emails PhaseConfig

	MinDelay   time.Duration
	MaxDelay   time.Duration
	MaxRetries int

	ThrottleStatusCode map[int]bool
}

func DefaultScraperConfig() ScraperConfig {
	return ScraperConfig{
		Cities: PhaseConfig{
			Parallelism: 2,
			Delay:       2 * time.Second,
			RandomDelay: 3 * time.Second,
			Timeout:     30 * time.Second,
		},
		Gyms: PhaseConfig{
			Parallelism: 4,
			Delay:       2 * time.Second,
			RandomDelay: 5 * time.Second,
			Timeout:     30 * time.Second,
		},
		Emails: PhaseConfig{
			Parallelism: 6,
			Delay:       2 * time.Second,
			RandomDelay: 5 * time.Second,
			Timeout:     30 * time.Second,
		},
		MinDelay:   time.Second,
		MaxDelay:   60 * time.Second,
		MaxRetries: 3,
		ThrottleStatusCode: map[int]bool{
			403: true,
			429: true,
			500: true,
			502: true,
			503: true,
			504: true,
		},
	}
}

func LoadScraperConfig() (ScraperConfig, error) {
	cfg := DefaultScraperConfig()

	integerValues := []struct {
		name      string
		target    *int
		allowZero bool
	}{
		{"SCRAPER_CITY_PARALLELISM", &cfg.Cities.Parallelism, false},
		{"SCRAPER_GYM_PARALLELISM", &cfg.Gyms.Parallelism, false},
		{"SCRAPER_EMAIL_PARALLELISM", &cfg.Emails.Parallelism, false},
		{"SCRAPER_MAX_RETRIES", &cfg.MaxRetries, true},
	}
	for _, value := range integerValues {
		if err := loadPositiveInt(value.name, value.target, value.allowZero); err != nil {
			return ScraperConfig{}, err
		}
	}

	if delay, ok, err := loadSeconds("SCRAPER_INITIAL_DELAY_SECONDS"); err != nil {
		return ScraperConfig{}, err
	} else if ok {
		cfg.Cities.Delay = delay
		cfg.Gyms.Delay = delay
		cfg.Emails.Delay = delay
	}
	if delay, ok, err := loadSeconds("SCRAPER_RANDOM_DELAY_SECONDS"); err != nil {
		return ScraperConfig{}, err
	} else if ok {
		cfg.Cities.RandomDelay = delay
		cfg.Gyms.RandomDelay = delay
		cfg.Emails.RandomDelay = delay
	}
	if timeout, ok, err := loadSeconds("SCRAPER_TIMEOUT_SECONDS"); err != nil {
		return ScraperConfig{}, err
	} else if ok {
		cfg.Cities.Timeout = timeout
		cfg.Gyms.Timeout = timeout
		cfg.Emails.Timeout = timeout
	}

	return cfg, nil
}

func loadPositiveInt(name string, target *int, allowZero bool) error {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || (!allowZero && value == 0) {
		return fmt.Errorf("%s must be a positive integer", name)
	}
	*target = value
	return nil
}

func loadSeconds(name string) (time.Duration, bool, error) {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return 0, false, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return 0, false, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return time.Duration(seconds) * time.Second, true, nil
}
