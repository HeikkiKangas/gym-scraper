# Planned Improvements

This document describes planned performance and reliability improvements for the web scraper. The focus is to fetch data faster while avoiding throttling, retry storms, and interrupted runs caused by temporary network failures.

The plan intentionally excludes persistent-cache changes and graceful-resume support.

## 1. Add measurement before tuning

### Goal

Make scraper performance observable before changing speed-related behavior. The current program reports the final email count and total elapsed time, but it does not show which phase is slowest or whether time is being lost to request delay, network latency, retries, throttling, duplicate work, or connection failures.

### Current behavior

The scraper has three major phases:

```go
cities := GetCities()
gyms := GetGyms(cities)
emails := GetEmails(gyms)
```

These phases run sequentially in `main()`.

### Required changes

Add a small metrics layer that records:

```text
total requests started
total requests completed
total request failures
HTTP status counts
request duration
phase duration
retry count
timeout count
throttling count
```

Create a new file:

```text
metrics.go
```

Suggested structs:

```go
type PhaseMetrics struct {
	Name             string
	StartedAt        time.Time
	FinishedAt       time.Time
	RequestsStarted  int64
	RequestsFinished int64
	RequestsFailed   int64
	Retries           int64
	Timeouts          int64
	StatusCounts      map[int]int64
	Latencies         []time.Duration
	mu                sync.Mutex
}

type ScraperMetrics struct {
	Cities PhaseMetrics
	Gyms   PhaseMetrics
	Emails PhaseMetrics
}
```

Add helper methods:

```go
func (m *PhaseMetrics) Start()
func (m *PhaseMetrics) Finish()
func (m *PhaseMetrics) RecordRequestStart()
func (m *PhaseMetrics) RecordResponse(statusCode int, duration time.Duration)
func (m *PhaseMetrics) RecordFailure(err error)
func (m *PhaseMetrics) RecordRetry()
func (m *PhaseMetrics) PrintSummary()
```

### Required Colly hook changes

Use these Colly hooks in `GetCities`, `GetGyms`, and `GetEmails`:

```go
c.OnRequest(func(r *colly.Request) {
	metrics.RecordRequestStart()
	r.Ctx.Put("startedAt", time.Now())
})

c.OnResponse(func(r *colly.Response) {
	startedAt := r.Ctx.GetAny("startedAt").(time.Time)
	metrics.RecordResponse(r.StatusCode, time.Since(startedAt))
})

c.OnError(func(r *colly.Response, err error) {
	metrics.RecordFailure(err)
})
```

The current `OnError` handlers should continue logging useful details, but they should also update metrics.

### Required output changes

At the end of each phase, print a compact summary:

```text
Phase: gyms
Requests: 248 completed, 3 failed, 5 retried
Status: 200=245, 500=2, 429=1
Latency: avg=820ms, p95=2.4s
Elapsed: 58.2s
```

### Acceptance criteria

The scraper should report:

```text
Collecting cities
Cities found: N
City phase elapsed: X seconds

Collecting gyms
Gym URLs found: N
Gym phase elapsed: X seconds

Collecting emails
Emails found: N
Email phase elapsed: X seconds

Total elapsed: X seconds
```

The metrics should make it clear whether the scraper is slow because of:

```text
too much delay
too much throttling
too many retries
slow server responses
connection failures
duplicate work
```

## 2. Replace fixed delays with adaptive rate limiting

### Goal

Fetch faster when the site is healthy, but slow down automatically when throttling or unstable connections appear.

### Current behavior

The scraper currently uses hardcoded constants:

```go
const PARALLELISM = 4
const DELAY = 60
const RANDOM_DELAY = 120
const TIMEOUT = 120
```

These are applied to both gym discovery and email fetching. This means each request can wait a long time before execution, even if the target server is responding quickly.

### Required changes

Create a configuration struct:

```go
type ScraperConfig struct {
	CityParallelism  int
	GymParallelism   int
	EmailParallelism int

	InitialDelay       time.Duration
	MinDelay           time.Duration
	MaxDelay           time.Duration
	RandomDelay        time.Duration
	Timeout            time.Duration
	MaxRetries         int
	ThrottleStatusCode map[int]bool
}
```

Replace hardcoded constants with configuration values.

Suggested default values:

```go
ScraperConfig{
	CityParallelism:  2,
	GymParallelism:   6,
	EmailParallelism: 6,

	InitialDelay: 2 * time.Second,
	MinDelay:     1 * time.Second,
	MaxDelay:     60 * time.Second,
	RandomDelay:  5 * time.Second,
	Timeout:      30 * time.Second,
	MaxRetries:   3,
	ThrottleStatusCode: map[int]bool{
		403: true,
		429: true,
		500: true,
		502: true,
		503: true,
		504: true,
	},
}
```

### Required runtime configuration

Add environment-variable support:

```text
SCRAPER_CITY_PARALLELISM
SCRAPER_GYM_PARALLELISM
SCRAPER_EMAIL_PARALLELISM
SCRAPER_INITIAL_DELAY_SECONDS
SCRAPER_RANDOM_DELAY_SECONDS
SCRAPER_TIMEOUT_SECONDS
SCRAPER_MAX_RETRIES
```

This avoids recompiling the program for every tuning run.

Example:

```bash
SCRAPER_GYM_PARALLELISM=8 SCRAPER_INITIAL_DELAY_SECONDS=2 go run .
```

### Required adaptive behavior

Implement an adaptive limiter that can increase or decrease request pressure.

Create a new file:

```text
rate_limiter.go
```

Suggested behavior:

```text
on successful 2xx response:
  keep delay stable
  optionally reduce delay slowly after several healthy responses

on 429:
  slow down immediately
  honor Retry-After if available

on 403:
  slow down aggressively
  avoid increasing parallelism

on 500/502/503/504:
  slow down moderately
  retry with backoff

on timeout or connection reset:
  reduce concurrency temporarily or increase delay
```

The rate limiter should not try to bypass throttling. It should respect signs that the server is overloaded or intentionally limiting requests.

### Required implementation details

Colly's `LimitRule` is static after setup, so there are two practical approaches.

#### Option A: conservative Colly limit plus adaptive retry delays

Keep Colly parallelism fixed but control retry timing through backoff.

This is simpler and safer.

```go
c.Limit(&colly.LimitRule{
	DomainGlob:  "*",
	Parallelism: cfg.EmailParallelism,
	Delay:       cfg.InitialDelay,
	RandomDelay: cfg.RandomDelay,
})
```

Then use adaptive delay inside retry logic.

#### Option B: custom worker pool

Replace Colly async scheduling with explicit workers and a shared adaptive limiter.

This is more flexible and pairs better with the pipeline step.

```go
limiter.Wait(ctx)
fetch(url)
limiter.Observe(statusCode, err)
```

### Acceptance criteria

The scraper should:

```text
run faster than the current 60s + 120s random delay setup
avoid retry storms
slow down when 429/403/5xx errors increase
print throttling-related metrics
continue processing other URLs when one URL fails
```

## 3. Add retry logic for interrupted connections

### Goal

Recover from temporary network failures without restarting the whole scraper.

### Current behavior

The current `OnError` handlers only print errors:

```go
c.OnError(func(r *colly.Response, e error) {
	fmt.Println("Request URL:", r.Request.URL, "\nError:", e)
})
```

This pattern is used in city, gym, and email scraping.

### Required changes

Create a retry helper:

```text
retry.go
```

Suggested API:

```go
func ShouldRetry(statusCode int, err error) bool
func RetryDelay(attempt int, baseDelay time.Duration) time.Duration
func VisitWithRetry(c *colly.Collector, url string, cfg ScraperConfig, metrics *PhaseMetrics) error
```

### Retryable failures

Retry these:

```text
network timeout
connection reset
temporary DNS failure
HTTP 429
HTTP 500
HTTP 502
HTTP 503
HTTP 504
```

Do not retry these:

```text
HTTP 400
HTTP 401
HTTP 404
invalid URL
parser failure
empty response that still returned 200
```

### Required attempt tracking

Use Colly request context to track attempts:

```go
attemptAny := r.Ctx.GetAny("attempt")
attempt, _ := attemptAny.(int)
```

Before retrying:

```go
if attempt < cfg.MaxRetries {
	nextAttempt := attempt + 1
	delay := RetryDelay(nextAttempt, cfg.InitialDelay)

	time.Sleep(delay)

	ctx := colly.NewContext()
	ctx.Put("attempt", nextAttempt)

	_ = r.Request.Visit(r.Request.URL.String())
}
```

A cleaner approach is to avoid retrying directly inside `OnError` and instead perform retries in a wrapper function that controls visits.

### Backoff requirements

Use exponential backoff with jitter:

```go
func RetryDelay(attempt int, base time.Duration) time.Duration {
	multiplier := 1 << attempt
	jitter := time.Duration(rand.Int63n(int64(base)))
	return time.Duration(multiplier)*base + jitter
}
```

Example delays:

```text
attempt 1: 2-4s
attempt 2: 4-8s
attempt 3: 8-16s
```

### Retry-After support

For `429`, inspect the `Retry-After` header:

```go
retryAfter := r.Headers.Get("Retry-After")
```

Support both formats:

```text
seconds
HTTP date
```

If present, use that delay instead of local backoff.

### Required failure reporting

After max retries are exhausted, record:

```text
failed URL
last status code
last error
attempt count
phase name
```

Do not terminate the full scraper because one gym page failed.

### Acceptance criteria

The scraper should survive:

```text
temporary connection reset
timeout on one URL
single 500 response
single 429 response
```

The final output should include:

```text
Retries: N
Failed URLs: N
```

## 4. De-duplicate URLs before fetching gym pages

### Goal

Avoid unnecessary duplicate gym-detail requests.

### Current behavior

`GetGyms()` appends every parsed gym URL directly into a slice:

```go
gyms = append(gyms, url)
```

There is no uniqueness check at this point.

### Required changes

Add a URL normalization helper:

```text
urls.go
```

Suggested functions:

```go
func NormalizeURL(raw string) (string, bool)
func AddUniqueURL(seen map[string]struct{}, urls *[]string, raw string) bool
```

### Normalization rules

Apply these rules before storing a gym URL:

```text
trim whitespace
ignore empty URLs
resolve relative URLs against https://kuntosali.fi
remove URL fragments
normalize trailing slash
lowercase scheme and host
keep path casing unchanged
```

Example:

```go
func NormalizeURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	base, _ := url.Parse("https://kuntosali.fi")
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}

	absolute := base.ResolveReference(parsed)
	absolute.Fragment = ""
	absolute.Scheme = strings.ToLower(absolute.Scheme)
	absolute.Host = strings.ToLower(absolute.Host)
	absolute.Path = strings.TrimRight(absolute.Path, "/") + "/"

	return absolute.String(), true
}
```

### Required `GetGyms()` change

Replace:

```go
gyms := []string{}
var mu sync.Mutex
```

with:

```go
gyms := []string{}
seenGyms := map[string]struct{}{}
var mu sync.Mutex
```

Replace the append logic:

```go
mu.Lock()
gyms = append(gyms, url)
mu.Unlock()
```

with:

```go
normalized, ok := NormalizeURL(url)
if !ok {
	return
}

mu.Lock()
if _, exists := seenGyms[normalized]; !exists {
	seenGyms[normalized] = struct{}{}
	gyms = append(gyms, normalized)
}
mu.Unlock()
```

### Required parser-level change

`parseGymURLsFromHTML()` also returns raw URLs from HTML.

Update it to normalize and de-duplicate URLs too, so tests and production behavior match.

### Acceptance criteria

Given these inputs:

```text
https://kuntosali.fi/salit/testi
https://kuntosali.fi/salit/testi/
 https://kuntosali.fi/salit/testi/#section
/salit/testi/
```

The scraper should fetch only one final normalized URL:

```text
https://kuntosali.fi/salit/testi/
```

## 5. Replace `slices.Contains` with maps for faster de-duplication

### Goal

Make uniqueness checks constant-time and reduce lock contention.

### Current behavior

Email collection uses:

```go
exists := slices.Contains(emails, email)
if !exists {
	emails = append(emails, email)
}
```

This happens inside a mutex-protected section.

Parser-level email extraction also uses `slices.Contains`.

### Required changes in `GetEmails()`

Replace:

```go
emails := []string{}
var mu sync.Mutex
```

with:

```go
emails := []string{}
seenEmails := map[string]struct{}{}
var mu sync.Mutex
```

Replace:

```go
mu.Lock()
exists := slices.Contains(emails, email)
if !exists {
	emails = append(emails, email)
}
mu.Unlock()
```

with:

```go
mu.Lock()
if _, exists := seenEmails[email]; !exists {
	seenEmails[email] = struct{}{}
	emails = append(emails, email)
}
mu.Unlock()
```

### Required changes in `parseEmailsFromHTML()`

Replace:

```go
emails := []string{}
```

with:

```go
emails := []string{}
seen := map[string]struct{}{}
```

Replace:

```go
if ok && !slices.Contains(emails, email) {
	emails = append(emails, email)
}
```

with:

```go
if ok {
	if _, exists := seen[email]; !exists {
		seen[email] = struct{}{}
		emails = append(emails, email)
	}
}
```

### Required import cleanup

After replacing `slices.Contains`, remove:

```go
"slices"
```

from `main.go` and `parsers.go` if no longer used.

### Acceptance criteria

The scraper should:

```text
produce the same unique email list
avoid O(n) duplicate checks
avoid importing slices for duplicate detection
perform better as the result list grows
```

## 6. Pipeline gym discovery and email scraping

### Goal

Start fetching gym detail pages as soon as gym URLs are discovered instead of waiting for every city page to finish.

### Current behavior

The scraper currently waits for all cities, then all gym URLs, then all emails:

```go
cities := GetCities()
gyms := GetGyms(cities)
emails := GetEmails(gyms)
```

Email fetching does not begin until `GetGyms()` returns.

### Required changes

Refactor the scraper into a producer-consumer pipeline:

```text
GetCities()
  -> city workers fetch city pages
  -> gym URL channel
  -> gym workers fetch gym pages
  -> email channel
  -> result collector
```

### Required new types

Create:

```go
type GymResult struct {
	URL    string
	Email  string
	Err    error
	Status int
}

type ScrapeResult struct {
	Emails     []string
	FailedURLs []string
	Metrics    ScraperMetrics
}
```

### Required function changes

Replace:

```go
func GetGyms(cities []string) []string
func GetEmails(gyms []string) []string
```

with pipeline-oriented functions:

```go
func ProduceGymURLs(ctx context.Context, cities []string, out chan<- string, cfg ScraperConfig, metrics *PhaseMetrics)

func ConsumeGymURLs(ctx context.Context, in <-chan string, out chan<- string, cfg ScraperConfig, metrics *PhaseMetrics)

func CollectEmails(in <-chan string) []string
```

Or create one orchestrating function:

```go
func ScrapeEmails(ctx context.Context, cfg ScraperConfig) ScrapeResult
```

### Required channel design

Use bounded channels to create backpressure:

```go
gymURLCh := make(chan string, 100)
emailCh := make(chan string, 100)
```

Bounded channels prevent the scraper from discovering many URLs faster than it can process them.

### Required worker behavior

City workers:

```text
fetch city page
parse gym URLs
normalize URL
deduplicate URL
send unique URL to gymURLCh
```

Gym workers:

```text
receive gym URL
fetch gym page with retry
parse emails
send emails to emailCh
```

Collector:

```text
receive email
deduplicate email using map
append unique emails
```

### Required synchronization

Use:

```go
var cityWG sync.WaitGroup
var gymWG sync.WaitGroup
```

Close channels in the right order:

```go
go func() {
	cityWG.Wait()
	close(gymURLCh)
}()

go func() {
	gymWG.Wait()
	close(emailCh)
}()
```

### Required safety behavior

The pipeline must not fail globally because one URL fails.

For each failed gym URL:

```text
record failure
continue processing remaining URLs
```

### Acceptance criteria

The scraper should:

```text
begin fetching gym detail pages before all city pages are complete
avoid duplicate gym-page requests
avoid unbounded memory growth
continue after individual URL failures
return the same final email format
```

## 7. Use separate rate limits per URL type

### Goal

Avoid using the same speed profile for city pages and gym pages. City pages and gym-detail pages have different volumes and should be tuned independently.

### Current behavior

Both `GetGyms()` and `GetEmails()` use the same constants:

```go
Parallelism: PARALLELISM
Delay:       DELAY * time.Second
RandomDelay: RANDOM_DELAY * time.Second
```

This makes the city-page phase and gym-detail phase equally slow even though their workloads are different.

### Required changes

Split config into per-phase settings:

```go
type PhaseConfig struct {
	Parallelism int
	Delay       time.Duration
	RandomDelay time.Duration
	Timeout     time.Duration
}

type ScraperConfig struct {
	Cities     PhaseConfig
	Gyms       PhaseConfig
	Emails     PhaseConfig
	MaxRetries int
}
```

Suggested defaults:

```go
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
```

Depending on the final pipeline design, `Gyms` and `Emails` may collapse into one gym-detail page fetch phase. In that case, keep a separate `GymPages` config.

### Required function signatures

Change:

```go
func GetGyms(cities []string) []string
func GetEmails(gyms []string) []string
```

to accept config:

```go
func GetGyms(cities []string, cfg PhaseConfig, metrics *PhaseMetrics) []string
func GetEmails(gyms []string, cfg PhaseConfig, metrics *PhaseMetrics) []string
```

Or, after pipeline refactor:

```go
func ScrapeEmails(ctx context.Context, cfg ScraperConfig) ScrapeResult
```

### Required Colly setup helper

Avoid duplicating collector setup. Create:

```go
func NewCollector(cfg PhaseConfig) *colly.Collector {
	c := colly.NewCollector(colly.Async(true), colly.CacheDir("./cache"))
	c.Limit(&colly.LimitRule{
		Parallelism: cfg.Parallelism,
		Delay:       cfg.Delay,
		RandomDelay: cfg.RandomDelay,
	})
	c.SetRequestTimeout(cfg.Timeout)
	return c
}
```

The current code repeats collector setup in both `GetGyms()` and `GetEmails()`.

### Acceptance criteria

The scraper should allow this kind of tuning:

```bash
SCRAPER_CITY_PARALLELISM=2 \
SCRAPER_GYM_PARALLELISM=8 \
SCRAPER_TIMEOUT_SECONDS=30 \
go run .
```

Metrics should report each phase separately so the settings can be tuned independently.

## 8. Lower timeout and rely on retries

### Goal

Avoid waiting up to two minutes for a broken or stalled connection.

### Current behavior

The timeout is currently:

```go
const TIMEOUT = 120
```

and this timeout is applied to all collectors.

### Required changes

Change timeout from a global constant to config:

```go
Timeout: 30 * time.Second
```

Apply it per phase:

```go
c.SetRequestTimeout(cfg.Timeout)
```

### Required timeout policy

Use:

```text
default timeout: 30s
minimum recommended timeout: 15s
maximum recommended timeout: 60s
```

Avoid returning to `120s` unless metrics show the target site regularly responds slowly but successfully.

### Required retry integration

Timeouts should not simply fail the URL immediately. They should feed into the retry policy:

```text
timeout on attempt 1 -> retry after short backoff
timeout on attempt 2 -> retry after longer backoff
timeout on final attempt -> mark URL failed and continue
```

### Required logging

When a timeout occurs, log:

```text
URL
phase
attempt number
timeout duration
next retry delay
```

Example:

```text
timeout phase=email url=https://kuntosali.fi/salit/example/ attempt=1 timeout=30s retry_in=4s
```

### Acceptance criteria

The scraper should:

```text
not block for 120s on one failed request
retry temporary timeout failures
continue after final retry failure
show timeout counts in metrics
complete faster under unstable network conditions
```

## Configuration target

Use this as the initial runtime target, not as permanent hardcoded constants:

```go
CityParallelism  = 2
GymParallelism   = 6
EmailParallelism = 6
Delay            = 2 * time.Second
RandomDelay      = 5 * time.Second
Timeout          = 30 * time.Second
MaxRetries       = 3
```

The important change is not just making the numbers faster. The important change is making the scraper measurable, configurable, retry-aware, and adaptive so it improves throughput without ignoring throttling signals.
