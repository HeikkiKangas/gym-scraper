# gym-scraper

Small Go CLI scraper that collects gym contact emails from `kuntosali.fi`.

## What It Does
- Fetches city pages from the site.
- Collects gym page URLs per city.
- Extracts and validates email addresses from gym info sections.
- Writes unique results to `emails.txt`.

## Requirements
- Go `1.25.5` (see `go.mod`)
- Network access to `https://kuntosali.fi`

## Run Locally
```bash
go run .
```

The scraper defaults to a 30-second request timeout and retries transient failures
up to three times with exponential backoff. Tune a run without recompiling:

```bash
SCRAPER_GYM_PARALLELISM=8 SCRAPER_INITIAL_DELAY_SECONDS=2 go run .
```

Supported variables:

- `SCRAPER_CITY_PARALLELISM`
- `SCRAPER_GYM_PARALLELISM`
- `SCRAPER_EMAIL_PARALLELISM`
- `SCRAPER_INITIAL_DELAY_SECONDS`
- `SCRAPER_RANDOM_DELAY_SECONDS`
- `SCRAPER_TIMEOUT_SECONDS`
- `SCRAPER_MAX_RETRIES`

A timeout between 15 and 60 seconds is recommended. Timeout, throttle, retry,
status, latency, and failed-URL metrics are printed separately for each phase.

Expected output includes progress logs:
- `Collecting cities`
- `Collecting gyms`
- `Collecting emails`
- total emails found and elapsed time

Results are written to:
- `emails.txt`

## Build
```bash
go build -o gym-scraper
```

Run the binary:
```bash
./gym-scraper
```

## Development Commands
```bash
go fmt ./...
go vet ./...
go test ./...
```

If your environment has a read-only default Go cache, use:
```bash
GOCACHE=/tmp/go-build go test ./...
```

## Notes
- Scraping is asynchronous with controlled parallelism and request delays.
- A temporary `./cache` directory is used during scraping and removed at the end.
- If writing `emails.txt` or removing cache fails, the program exits with an error.
