package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// APIError is a non-200 response from a chat endpoint. It carries whatever the
// provider said about when the request may be repeated, so the caller can wait
// exactly that long instead of guessing.
type APIError struct {
	StatusCode int
	Body       string
	Provider   string
	// RetryAfter is how long to wait before repeating the request. Only
	// meaningful when HasHint is true.
	RetryAfter time.Duration
	HasHint    bool
}

func (e *APIError) Error() string {
	return fmt.Sprintf("chat API error (HTTP %d): %s", e.StatusCode, e.Body)
}

// RateLimited reports whether the endpoint refused because of a usage limit
// rather than a transient fault.
func (e *APIError) RateLimited() bool {
	return e.StatusCode == http.StatusTooManyRequests
}

const (
	// retryBaseDelay is the wait before the first retry; each subsequent
	// attempt doubles it up to retryMaxDelay.
	retryBaseDelay = 3 * time.Second
	retryMaxDelay  = 60 * time.Second
	// retryHintMargin is added to a provider-supplied reset time so the retry
	// lands just after the window opens rather than exactly on the boundary.
	retryHintMargin = time.Second
)

// BackoffDelay returns the wait before the given 1-based retry attempt:
// 3s, 6s, 12s, 24s, 48s, then 60s, with +/-20% jitter so concurrent clients
// don't converge on the same instant.
func BackoffDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := retryBaseDelay
	if attempt > 1 {
		shift := float64(attempt - 1)
		if shift > 10 {
			shift = 10
		}
		d = time.Duration(float64(retryBaseDelay) * math.Pow(2, shift))
	}
	if d > retryMaxDelay {
		d = retryMaxDelay
	}
	jitter := 1 + (rand.Float64()*0.4 - 0.2)
	return time.Duration(float64(d) * jitter)
}

// Retryable reports whether repeating the identical request could plausibly
// succeed. A cancelled context never is: that is the user pressing esc.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusRequestTimeout, // 408
			http.StatusConflict,           // 409
			http.StatusTooEarly,           // 425
			http.StatusTooManyRequests:    // 429
			return true
		}
		return apiErr.StatusCode >= 500
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.ENETDOWN) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsTemporary || dnsErr.IsTimeout || dnsErr.IsNotFound
	}

	// Streams that die partway leave a plain wrapped error from the scanner.
	msg := err.Error()
	for _, frag := range []string{
		"connection reset",
		"broken pipe",
		"unexpected EOF",
		"EOF",
		"timeout",
		"timed out",
		"no such host",
		"connection refused",
		"server closed",
	} {
		if strings.Contains(msg, frag) {
			return true
		}
	}
	return false
}

var (
	// "try again in 1.5s", "retry in 6m0s", "try again in 4 hours"
	tryAgainRe = regexp.MustCompile(`(?i)(?:try again|retry|retrying)\s+(?:in|after)\s+([0-9]+(?:\.[0-9]+)?)\s*(ms|milliseconds?|s|secs?|seconds?|m|mins?|minutes?|h|hrs?|hours?)`)
	// "resets at 2026-07-21T18:00:00Z"
	resetsAtRe = regexp.MustCompile(`(?i)(?:resets?|available again|limit will reset)\s+(?:at|on)?\s*([0-9]{4}-[0-9]{2}-[0-9]{2}[Tt ][0-9:.+\-Zz]+)`)
	// Compact Go-ish duration: "6m0s", "1h30m", "88ms"
	goDurationRe = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(ms|us|µs|ns|s|m|h)([0-9]+(\.[0-9]+)?(ms|us|µs|ns|s|m|h))*$`)
)

// anthropicResetHeaders and openaiResetHeaders are checked in order after
// Retry-After. Anthropic reports RFC3339 instants; OpenAI reports durations.
var (
	anthropicResetHeaders = []string{
		"anthropic-ratelimit-unified-reset",
		"anthropic-ratelimit-tokens-reset",
		"anthropic-ratelimit-requests-reset",
	}
	openaiResetHeaders = []string{
		"x-ratelimit-reset-tokens",
		"x-ratelimit-reset-requests",
	}
)

// ParseRetryAfter extracts how long to wait before repeating a request from a
// response's headers, falling back to the error body. It returns false when the
// provider gave no usable hint, or when every hint points into the past.
func ParseRetryAfter(h http.Header, body string, now time.Time) (time.Duration, bool) {
	if d, ok := parseRetryAfterHeader(h.Get("Retry-After"), now); ok {
		return d, true
	}

	// Anthropic: RFC3339 instants (or, on some deployments, unix seconds).
	var best time.Duration
	found := false
	for _, name := range anthropicResetHeaders {
		v := strings.TrimSpace(h.Get(name))
		if v == "" {
			continue
		}
		d, ok := parseInstantOrEpoch(v, now)
		if !ok {
			continue
		}
		if !found || d < best {
			best, found = d, true
		}
	}
	if found {
		return best, true
	}

	// OpenAI: durations like "6m0s", "1s", "88ms".
	for _, name := range openaiResetHeaders {
		v := strings.TrimSpace(h.Get(name))
		if v == "" {
			continue
		}
		d, ok := parseCompactDuration(v)
		if !ok || d < 0 {
			continue
		}
		if !found || d < best {
			best, found = d, true
		}
	}
	if found {
		return best, true
	}

	// OpenRouter and friends: unix epoch, seconds or milliseconds.
	if v := strings.TrimSpace(h.Get("x-ratelimit-reset")); v != "" {
		if d, ok := parseInstantOrEpoch(v, now); ok {
			return d, true
		}
	}

	return parseRetryFromBody(body, now)
}

func parseRetryAfterHeader(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs * float64(time.Second)), true
	}
	if t, err := http.ParseTime(v); err == nil {
		d := t.Sub(now)
		if d < 0 {
			return 0, false
		}
		return d, true
	}
	return 0, false
}

// parseInstantOrEpoch reads an RFC3339 timestamp or a unix epoch (seconds or
// milliseconds, discriminated by magnitude) and returns the delay until then.
func parseInstantOrEpoch(v string, now time.Time) (time.Duration, bool) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		d := t.Sub(now)
		if d < 0 {
			return 0, false
		}
		return d, true
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	var t time.Time
	switch {
	case n > 1e12: // milliseconds since epoch
		t = time.UnixMilli(n)
	case n > 1e9: // seconds since epoch
		t = time.Unix(n, 0)
	default: // small integer: a relative number of seconds
		if n < 0 {
			return 0, false
		}
		return time.Duration(n) * time.Second, true
	}
	d := t.Sub(now)
	if d < 0 {
		return 0, false
	}
	return d, true
}

// parseCompactDuration accepts Go duration syntax plus a bare number of
// seconds, which some compatible endpoints emit.
func parseCompactDuration(v string) (time.Duration, bool) {
	if goDurationRe.MatchString(v) {
		if d, err := time.ParseDuration(v); err == nil {
			return d, true
		}
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil {
		return time.Duration(secs * float64(time.Second)), true
	}
	return 0, false
}

// parseRetryFromBody handles providers that only mention the reset window in
// the error message, which is where multi-hour subscription limits show up.
func parseRetryFromBody(body string, now time.Time) (time.Duration, bool) {
	if body == "" {
		return 0, false
	}
	if m := tryAgainRe.FindStringSubmatch(body); m != nil {
		n, err := strconv.ParseFloat(m[1], 64)
		if err == nil && n >= 0 {
			var unit time.Duration
			switch strings.ToLower(m[2]) {
			case "ms", "millisecond", "milliseconds":
				unit = time.Millisecond
			case "s", "sec", "secs", "second", "seconds":
				unit = time.Second
			case "m", "min", "mins", "minute", "minutes":
				unit = time.Minute
			case "h", "hr", "hrs", "hour", "hours":
				unit = time.Hour
			}
			if unit > 0 {
				return time.Duration(n * float64(unit)), true
			}
		}
	}
	if m := resetsAtRe.FindStringSubmatch(body); m != nil {
		raw := strings.Replace(strings.TrimSpace(m[1]), " ", "T", 1)
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			if d := t.Sub(now); d >= 0 {
				return d, true
			}
		}
	}
	return 0, false
}

// RetryHint returns the wait a provider asked for, plus a small margin so the
// request lands after the window opens rather than on its boundary.
func RetryHint(err error) (time.Duration, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.HasHint {
		return apiErr.RetryAfter + retryHintMargin, true
	}
	return 0, false
}

// IsRateLimit reports whether the failure was a usage-limit rejection, which
// the UI labels differently from a generic failure.
func IsRateLimit(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.RateLimited()
}
