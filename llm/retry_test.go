package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		headers map[string]string
		body    string
		want    time.Duration
		wantOK  bool
	}{
		{
			name:    "retry-after seconds",
			headers: map[string]string{"Retry-After": "30"},
			want:    30 * time.Second,
			wantOK:  true,
		},
		{
			name:    "retry-after fractional seconds",
			headers: map[string]string{"Retry-After": "1.5"},
			want:    1500 * time.Millisecond,
			wantOK:  true,
		},
		{
			name:    "retry-after http date",
			headers: map[string]string{"Retry-After": "Tue, 21 Jul 2026 12:02:00 GMT"},
			want:    2 * time.Minute,
			wantOK:  true,
		},
		{
			name:    "retry-after in the past is no hint",
			headers: map[string]string{"Retry-After": "Tue, 21 Jul 2026 11:00:00 GMT"},
			wantOK:  false,
		},
		{
			name:    "anthropic unified reset",
			headers: map[string]string{"anthropic-ratelimit-unified-reset": "2026-07-21T12:00:45Z"},
			want:    45 * time.Second,
			wantOK:  true,
		},
		{
			name: "anthropic picks the soonest reset",
			headers: map[string]string{
				"anthropic-ratelimit-tokens-reset":   "2026-07-21T12:05:00Z",
				"anthropic-ratelimit-requests-reset": "2026-07-21T12:01:00Z",
			},
			want:   time.Minute,
			wantOK: true,
		},
		{
			name:    "anthropic multi-hour reset",
			headers: map[string]string{"anthropic-ratelimit-unified-reset": "2026-07-21T17:00:00Z"},
			want:    5 * time.Hour,
			wantOK:  true,
		},
		{
			name:    "retry-after wins over reset headers",
			headers: map[string]string{"Retry-After": "10", "anthropic-ratelimit-unified-reset": "2026-07-21T13:00:00Z"},
			want:    10 * time.Second,
			wantOK:  true,
		},
		{
			name:    "openai duration header",
			headers: map[string]string{"x-ratelimit-reset-tokens": "6m0s"},
			want:    6 * time.Minute,
			wantOK:  true,
		},
		{
			name:    "openai millisecond header",
			headers: map[string]string{"x-ratelimit-reset-requests": "88ms"},
			want:    88 * time.Millisecond,
			wantOK:  true,
		},
		{
			name: "openai picks the soonest of the two",
			headers: map[string]string{
				"x-ratelimit-reset-tokens":   "6m0s",
				"x-ratelimit-reset-requests": "1s",
			},
			want:   time.Second,
			wantOK: true,
		},
		{
			name:    "generic epoch seconds",
			headers: map[string]string{"x-ratelimit-reset": fmt.Sprint(now.Add(90 * time.Second).Unix())},
			want:    90 * time.Second,
			wantOK:  true,
		},
		{
			name:    "generic epoch milliseconds",
			headers: map[string]string{"x-ratelimit-reset": fmt.Sprint(now.Add(90*time.Second).UnixMilli())},
			want:    90 * time.Second,
			wantOK:  true,
		},
		{
			name:   "body try again in seconds",
			body:   `{"error":{"message":"Rate limit reached. Please try again in 1.5s."}}`,
			want:   1500 * time.Millisecond,
			wantOK: true,
		},
		{
			name:   "body try again in hours",
			body:   `{"error":{"message":"You have hit your usage limit. Try again in 5 hours."}}`,
			want:   5 * time.Hour,
			wantOK: true,
		},
		{
			name:   "body resets at timestamp",
			body:   `{"error":{"message":"Usage limit exceeded; resets at 2026-07-21T16:58:00Z"}}`,
			want:   4*time.Hour + 58*time.Minute,
			wantOK: true,
		},
		{
			name:   "no hint at all",
			body:   `{"error":{"message":"internal server error"}}`,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range tt.headers {
				h.Set(k, v)
			}
			got, ok := ParseRetryAfter(h, tt.body, now)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (got %v)", ok, tt.wantOK, got)
			}
			if ok && got != tt.want {
				t.Errorf("delay = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"429", &APIError{StatusCode: 429}, true},
		{"408", &APIError{StatusCode: 408}, true},
		{"500", &APIError{StatusCode: 500}, true},
		{"502", &APIError{StatusCode: 502}, true},
		{"503", &APIError{StatusCode: 503}, true},
		{"400", &APIError{StatusCode: 400}, false},
		{"401", &APIError{StatusCode: 401}, false},
		{"403", &APIError{StatusCode: 403}, false},
		{"404", &APIError{StatusCode: 404}, false},
		{"422", &APIError{StatusCode: 422}, false},
		{"wrapped 429", fmt.Errorf("chat request: %w", &APIError{StatusCode: 429}), true},
		{"user cancelled", context.Canceled, false},
		{"cancelled wrapped", fmt.Errorf("chat request: %w", context.Canceled), false},
		{"deadline", context.DeadlineExceeded, true},
		{"connection reset", fmt.Errorf("reading stream: %w", syscall.ECONNRESET), true},
		{"broken pipe", syscall.EPIPE, true},
		{"unexpected eof", fmt.Errorf("reading stream: %w", io.ErrUnexpectedEOF), true},
		{"net timeout", &net.OpError{Op: "dial", Err: syscall.ETIMEDOUT}, true},
		{"plain message", errors.New("marshaling request: bad input"), false},
		{"text mentioning timeout", errors.New("chat request: net/http: timeout awaiting response headers"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Retryable(tt.err); got != tt.want {
				t.Errorf("Retryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestBackoffDelay(t *testing.T) {
	// Nominal ladder before jitter: 3, 6, 12, 24, 48, 60, 60, 60.
	nominal := []time.Duration{
		3 * time.Second, 6 * time.Second, 12 * time.Second, 24 * time.Second,
		48 * time.Second, 60 * time.Second, 60 * time.Second, 60 * time.Second,
	}
	for i, want := range nominal {
		attempt := i + 1
		for range 50 {
			got := BackoffDelay(attempt)
			lo := time.Duration(float64(want) * 0.8)
			hi := time.Duration(float64(want) * 1.2)
			if got < lo || got > hi {
				t.Fatalf("attempt %d: delay %v outside jitter band [%v, %v]", attempt, got, lo, hi)
			}
		}
	}
	// Never exceeds the cap plus jitter, however high the attempt count goes.
	if got := BackoffDelay(40); got > time.Duration(float64(retryMaxDelay)*1.2) {
		t.Errorf("attempt 40: delay %v exceeds capped band", got)
	}
}

func TestRetryHintAddsMargin(t *testing.T) {
	err := &APIError{StatusCode: 429, RetryAfter: 5 * time.Second, HasHint: true}
	got, ok := RetryHint(err)
	if !ok {
		t.Fatal("expected a hint")
	}
	if got != 6*time.Second {
		t.Errorf("hint = %v, want 6s (reset + 1s margin)", got)
	}

	if _, ok := RetryHint(&APIError{StatusCode: 500}); ok {
		t.Error("expected no hint when HasHint is false")
	}
	if _, ok := RetryHint(errors.New("boom")); ok {
		t.Error("expected no hint from a plain error")
	}
}

// The rate-limit headers must survive the round trip through the real client.
func TestStreamChatCompletionReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.Header().Set("x-ratelimit-reset-tokens", "6m0s")
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"error":{"message":"rate limit exceeded"}}`)
	}))
	defer srv.Close()

	_, err := StreamChatCompletion(context.Background(), ProviderCustom, "k", srv.URL, ChatRequest{
		Model:    "test",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected an error")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", apiErr.StatusCode)
	}
	if !apiErr.HasHint || apiErr.RetryAfter != 7*time.Second {
		t.Errorf("RetryAfter = %v (hint %v), want 7s", apiErr.RetryAfter, apiErr.HasHint)
	}
	if !Retryable(err) {
		t.Error("a 429 should be retryable")
	}
	if !IsRateLimit(err) {
		t.Error("a 429 should be flagged as a rate limit")
	}
	// The message wording is unchanged from before APIError existed.
	if !strings.Contains(err.Error(), "chat API error (HTTP 429)") {
		t.Errorf("unexpected message %q", err.Error())
	}
}

func TestStreamChatCompletionAuthErrorNotRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"message":"invalid api key"}}`)
	}))
	defer srv.Close()

	_, err := StreamChatCompletion(context.Background(), ProviderCustom, "k", srv.URL, ChatRequest{
		Model:    "test",
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if Retryable(err) {
		t.Error("a 401 must not be retried")
	}
}

func TestIsRateLimit(t *testing.T) {
	if !IsRateLimit(fmt.Errorf("wrapped: %w", &APIError{StatusCode: 429})) {
		t.Error("429 should be a rate limit")
	}
	if IsRateLimit(&APIError{StatusCode: 503}) {
		t.Error("503 is not a rate limit")
	}
	if IsRateLimit(errors.New("boom")) {
		t.Error("plain error is not a rate limit")
	}
}
