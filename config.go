package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type ScraperConfig struct {
	CityParallelism  int
	GymParallelism   int
	EmailParallelism int

	InitialDelay time.Duration
	MinDelay     time.Duration
	MaxDelay     time.Duration
	RandomDelay  time.Duration
	Timeout      time.Duration
	MaxRetries   int

	ThrottleStatusCode map[int]bool
}

func DefaultScraperConfig() ScraperConfig {
	return ScraperConfig{
		CityParallelism:  2,
		GymParallelism:   6,
		EmailParallelism: 6,
		InitialDelay:     2 * time.Second,
		MinDelay:         time.Second,
		MaxDelay:         60 * time.Second,
		RandomDelay:      5 * time.Second,
		Timeout:          30 * time.Second,
		MaxRetries:       3,
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
		name   string
		target *int
	}{
		{"SCRAPER_CITY_PARALLELISM", &cfg.CityParallelism},
		{"SCRAPER_GYM_PARALLELISM", &cfg.GymParallelism},
		{"SCRAPER_EMAIL_PARALLELISM", &cfg.EmailParallelism},
		{"SCRAPER_MAX_RETRIES", &cfg.MaxRetries},
	}
	for _, value := range integerValues {
		if err := loadPositiveInt(value.name, value.target, value.name == "SCRAPER_MAX_RETRIES"); err != nil {
			return ScraperConfig{}, err
		}
	}

	durationValues := []struct {
		name   string
		target *time.Duration
	}{
		{"SCRAPER_INITIAL_DELAY_SECONDS", &cfg.InitialDelay},
		{"SCRAPER_RANDOM_DELAY_SECONDS", &cfg.RandomDelay},
		{"SCRAPER_TIMEOUT_SECONDS", &cfg.Timeout},
	}
	for _, value := range durationValues {
		if err := loadSeconds(value.name, value.target); err != nil {
			return ScraperConfig{}, err
		}
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

func loadSeconds(name string, target *time.Duration) error {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return fmt.Errorf("%s must be a non-negative integer", name)
	}
	*target = time.Duration(seconds) * time.Second
	return nil
}
