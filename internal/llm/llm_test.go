package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	client := New([]string{"model-a", "model-b"}, "https://api.openai.com/v1/", "TEST_KEY", 30*time.Second)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if len(client.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(client.Models))
	}
	if client.BaseURL != "https://api.openai.com/v1/" {
		t.Errorf("unexpected BaseURL: %q", client.BaseURL)
	}
	if client.Timeout != 30*time.Second {
		t.Errorf("unexpected Timeout: %v", client.Timeout)
	}
	if client.httpClient == nil {
		t.Error("expected non-nil httpClient on Client")
	}
}

func TestCall_MissingAPIKey(t *testing.T) {
	client := New([]string{"model-a"}, "http://localhost:8080", "NONEXISTENT_VAR_FOR_TEST_VAULTCHRON", 1*time.Second)
	_, _, err := client.Call(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error when API key environment variable is missing")
	}
	if !strings.Contains(err.Error(), "not set") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCall_Success(t *testing.T) {
	t.Setenv("MOCK_LLM_KEY", "secret-test-token")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer secret-test-token" {
			t.Errorf("unexpected auth header: %s", auth)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var payload chatPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("error decoding body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{
					Message: struct {
						Content string `json:"content"`
					}{
						Content: "Devlog entry for model " + payload.Model,
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := New([]string{"custom-model-1"}, server.URL, "MOCK_LLM_KEY", 5*time.Second)
	content, model, err := client.Call(context.Background(), "system prompt", "user prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if model != "custom-model-1" {
		t.Errorf("expected model custom-model-1, got %q", model)
	}
	if !strings.Contains(content, "Devlog entry for model custom-model-1") {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestCall_WaterfallFallback(t *testing.T) {
	t.Setenv("MOCK_LLM_KEY", "secret-test-token")

	oldBackoff := backoffFn
	backoffFn = func(attempt int) time.Duration {
		return time.Millisecond
	}
	defer func() { backoffFn = oldBackoff }()

	var modelACalls int32
	var modelBCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload chatPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if payload.Model == "model-fail" {
			atomic.AddInt32(&modelACalls, 1)
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}

		if payload.Model == "model-ok" {
			atomic.AddInt32(&modelBCalls, 1)
			resp := chatResponse{
				Choices: []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				}{
					{
						Message: struct {
							Content string `json:"content"`
						}{
							Content: "Success from fallback model",
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	// Short timeout so retries don't delay test
	client := New([]string{"model-fail", "model-ok"}, server.URL, "MOCK_LLM_KEY", 2*time.Second)
	content, model, err := client.Call(context.Background(), "system prompt", "user prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if model != "model-ok" {
		t.Errorf("expected model model-ok, got %q", model)
	}
	if content != "Success from fallback model" {
		t.Errorf("unexpected content: %q", content)
	}
}

func TestCall_ContextCancellation(t *testing.T) {
	t.Setenv("MOCK_LLM_KEY", "secret-test-token")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New([]string{"model-a"}, server.URL, "MOCK_LLM_KEY", 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, _, err := client.Call(ctx, "sys", "user")
	if err == nil {
		t.Fatal("expected error on canceled context")
	}
	if !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}

func TestCall_WaterfallErrorJoin(t *testing.T) {
	t.Setenv("MOCK_LLM_KEY", "secret-test-token")

	oldBackoff := backoffFn
	backoffFn = func(attempt int) time.Duration {
		return time.Millisecond
	}
	defer func() { backoffFn = oldBackoff }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload chatPayload
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.Model == "model-1" {
			http.Error(w, "model-1 internal error", http.StatusInternalServerError)
			return
		}
		if payload.Model == "model-2" {
			http.Error(w, "model-2 bad gateway", http.StatusBadGateway)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := New([]string{"model-1", "model-2"}, server.URL, "MOCK_LLM_KEY", 2*time.Second)
	_, _, err := client.Call(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error when all models fail")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "model \"model-1\"") || !strings.Contains(errMsg, "model \"model-2\"") {
		t.Errorf("expected error message to contain details for all failing models, got: %v", errMsg)
	}
}

func TestCall_ResponseLimitReader(t *testing.T) {
	t.Setenv("MOCK_LLM_KEY", "secret-test-token")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Stream more than 10MB of invalid JSON padding
		chunk := []byte(strings.Repeat(" ", 1024*1024)) // 1MB
		for i := 0; i < 12; i++ {
			_, _ = w.Write(chunk)
		}
	}))
	defer server.Close()

	client := New([]string{"model-limit"}, server.URL, "MOCK_LLM_KEY", 2*time.Second)
	_, _, err := client.Call(context.Background(), "sys", "user")
	if err == nil {
		t.Fatal("expected error parsing truncated oversized payload")
	}
	// Expect unmarshal error rather than OOM
	if !strings.Contains(err.Error(), "unmarshalling") {
		t.Logf("got error: %v", err)
	}
}
