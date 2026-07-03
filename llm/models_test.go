package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchModelsOpenAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/models" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		resp := modelsResponse{
			Data: []struct {
				ID string `json:"id"`
			}{
				{ID: "gpt-5.5"},
				{ID: "gpt-5.4"},
				{ID: "gpt-5.4-mini"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	models, err := FetchModels(context.Background(), ProviderOpenAI, "test-key", srv.URL)
	if err != nil {
		t.Fatalf("FetchModels() returned error: %v", err)
	}
	if len(models) != 3 {
		t.Errorf("got %d models, want 3", len(models))
	}
	if models[0] != "gpt-5.5" {
		t.Errorf("models[0] = %q, want %q", models[0], "gpt-5.5")
	}
}

func TestFetchModelsAnthropic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		resp := modelsResponse{
			Data: []struct {
				ID string `json:"id"`
			}{
				{ID: "claude-sonnet-5"},
				{ID: "claude-opus-4-8"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	models, err := FetchModels(context.Background(), ProviderAnthropic, "test-key", srv.URL)
	if err != nil {
		t.Fatalf("FetchModels() returned error: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("got %d models, want 2", len(models))
	}
}

func TestFetchModelsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := FetchModels(context.Background(), ProviderOpenAI, "bad-key", srv.URL)
	if err == nil {
		t.Fatal("FetchModels() expected error for 401")
	}
}

func TestFetchModelsEmptyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := modelsResponse{Data: []struct {
			ID string `json:"id"`
		}{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	_, err := FetchModels(context.Background(), ProviderOpenAI, "key", srv.URL)
	if err == nil {
		t.Fatal("FetchModels() expected error for empty model list")
	}
}

func TestFetchModelsContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := FetchModels(ctx, ProviderOpenAI, "key", "http://localhost:1")
	if err == nil {
		t.Fatal("FetchModels() expected error for canceled context")
	}
}

func TestFetchModelsNetworkError(t *testing.T) {
	_, err := FetchModels(context.Background(), ProviderOpenAI, "key", "http://localhost:1")
	if err == nil {
		t.Fatal("FetchModels() expected error for network failure")
	}
}

func TestIsTextChatModel(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		// Match: current OpenAI models for agentic coding (mid-2026)
		{"gpt-5.5", true},     // flagship
		{"gpt-5.4", true},     // balanced
		{"gpt-5.4-mini", true}, // efficiency
		// Case insensitive
		{"GPT-5.5", true},
		{"Gpt-5.4", true},
		// Excluded: real but non-text or non-GPT models
		{"claude-fable-5", false},
		{"claude-opus-4-8", false},
		{"claude-sonnet-5", false},
		{"claude-haiku-4-5", false},
		{"claude-sonnet-4.6", false},
		{"dall-e-3", false},
		{"whisper-1", false},
		{"text-embedding-3-small", false},
		{"text-embedding-3-large", false},
		// Excluded: gpt- prefix but doesn't fit digit/o-digit pattern
		{"gpt-", false},
		{"gpt-foo", false},
		{"gpt-o", false},
		{"gpt-o-foo", false},
		{"", false},
	}

	for _, tt := range tests {
		got := IsTextChatModel(tt.id)
		if got != tt.want {
			t.Errorf("IsTextChatModel(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestFetchModelsCustom(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer custom-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		resp := modelsResponse{
			Data: []struct {
				ID string `json:"id"`
			}{
				{ID: "my-model-v1"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	models, err := FetchModels(context.Background(), ProviderCustom, "custom-key", srv.URL)
	if err != nil {
		t.Fatalf("FetchModels() returned error: %v", err)
	}
	if len(models) != 1 {
		t.Errorf("got %d models, want 1", len(models))
	}
}
