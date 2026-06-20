package main

import "github.com/gocolly/colly"

func NewCollector(cfg PhaseConfig) *colly.Collector {
	c := colly.NewCollector(colly.CacheDir("./cache"))
	_ = c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: cfg.Parallelism,
		Delay:       cfg.Delay,
		RandomDelay: cfg.RandomDelay,
	})
	c.SetRequestTimeout(cfg.Timeout)
	return c
}
