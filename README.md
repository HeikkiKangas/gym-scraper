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
