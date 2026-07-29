package localllm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultURL = "http://localhost:11434"

// Client talks to a local Ollama daemon.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient(baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 0}, // pulls can take a long time
	}
}

// Ping checks whether Ollama is reachable.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient(10 * time.Second).Do(req)
	if err != nil {
		return fmt.Errorf("ollama not reachable at %s — install/start Ollama from ollama.com: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ollama returned HTTP %d", resp.StatusCode)
	}
	return nil
}

type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// ListInstalled returns names of models already pulled.
func (c *Client) ListInstalled(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient(15 * time.Second).Do(req)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list models HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tr tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(tr.Models))
	for _, m := range tr.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// ProgressFunc is called with status updates during Pull.
type ProgressFunc func(status string, completed, total int64)

// Pull downloads/installs a model via Ollama. Blocks until done.
func (c *Client) Pull(ctx context.Context, model string, onProgress ProgressFunc) error {
	payload, _ := json.Marshal(map[string]any{
		"name":   model,
		"stream": true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/pull", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("pull %s: %w", model, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pull %s HTTP %d: %s", model, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var ev struct {
			Status    string `json:"status"`
			Error     string `json:"error"`
			Completed int64  `json:"completed"`
			Total     int64  `json:"total"`
		}
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Error != "" {
			return fmt.Errorf("pull %s: %s", model, ev.Error)
		}
		if onProgress != nil {
			onProgress(ev.Status, ev.Completed, ev.Total)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("pull %s stream: %w", model, err)
	}
	return nil
}

func (c *Client) httpClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}
