package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Client is a minimal OpenAI-compatible LLM client that supports a
// waterfall fallback chain across configured models.
type Client struct {
	Models     []string // waterfall order
	BaseURL    string
	APIKeyEnv  string
	Timeout    time.Duration
	httpClient *http.Client
}

// New creates a Client from model, endpoint, and authentication config.
func New(models []string, baseURL, apiKeyEnv string, timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	return &Client{
		Models:     models,
		BaseURL:    baseURL,
		APIKeyEnv:  apiKeyEnv,
		Timeout:    timeout,
		httpClient: &http.Client{Timeout: timeout},
	}
}

// chatPayload mirrors the OpenAI chat/completions request body.
type chatPayload struct {
	Model       string    `json:"model"`
	Messages    []chatMsg `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float32   `json:"temperature"`
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// maxRetries is the number of retry attempts for transient HTTP errors
// (503, 429, 502, 504) within a single model call.
const maxRetries = 3

// retryableStatus codes that warrant a retry with backoff.
var retryableStatus = map[int]bool{
	http.StatusTooManyRequests:    true, // 429
	http.StatusBadGateway:         true, // 502
	http.StatusServiceUnavailable: true, // 503
	http.StatusGatewayTimeout:     true, // 504
}

// Call executes a prompt through the waterfall chain of models.
// Returns the first successful content string and the model that produced it.
// On 429 (rate limit) or 503 (service unavailable), falls through to the next
// model in the chain after exhausting retries with exponential backoff.
func (c *Client) Call(ctx context.Context, systemPrompt, userPrompt string) (string, string, error) {
	apiKey := os.Getenv(c.APIKeyEnv)
	if apiKey == "" {
		return "", "", fmt.Errorf("environment variable %s is not set", c.APIKeyEnv)
	}

	var errs []error
	for _, model := range c.Models {
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
		content, err := c.callModel(ctx, model, systemPrompt, userPrompt, apiKey)
		if err == nil && content != "" {
			return content, model, nil
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("model %q: %w", model, err))
		}
		// On any error (including 429/503 after retries), fall through to next model
	}
	if len(errs) > 0 {
		return "", "", fmt.Errorf("all models in waterfall chain failed: %w", errors.Join(errs...))
	}
	return "", "", fmt.Errorf("all models in waterfall chain failed: no models configured")
}

func (c *Client) callModel(ctx context.Context, model, systemPrompt, userPrompt, apiKey string) (string, error) {
	payload := chatPayload{
		Model: model,
		Messages: []chatMsg{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens:   8192,
		Temperature: 0.3,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshaling payload: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimRight(c.BaseURL, "/"))

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request: %w", err)
			if attempt < maxRetries {
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(backoffFn(attempt)):
				}
				continue
			}
			return "", lastErr
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		_ = resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("reading response: %w", err)
			if attempt < maxRetries {
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(backoffFn(attempt)):
				}
				continue
			}
			return "", lastErr
		}

		if retryableStatus[resp.StatusCode] {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 500)]))
			if attempt < maxRetries {
				select {
				case <-ctx.Done():
					return "", ctx.Err()
				case <-time.After(backoffFn(attempt)):
				}
				continue
			}
			return "", lastErr
		}

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 500)]))
		}

		var chatResp chatResponse
		if err := json.Unmarshal(respBody, &chatResp); err != nil {
			return "", fmt.Errorf("unmarshalling response: %w", err)
		}

		if len(chatResp.Choices) == 0 {
			return "", fmt.Errorf("no choices in response")
		}

		return chatResp.Choices[0].Message.Content, nil
	}
	return "", lastErr
}

var backoffFn = backoff

// backoff returns an exponential backoff duration for the given attempt index.
// Sequence: 2s, 4s, 8s.
func backoff(attempt int) time.Duration {
	return time.Duration(2<<uint(attempt)) * time.Second
}
