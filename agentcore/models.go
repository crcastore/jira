package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// OllamaModelCatalog discovers tool-capable models from the Ollama /api/tags
// endpoint derived from an OpenAI-compatible base URL.
type OllamaModelCatalog struct {
	baseURL string
	client  *http.Client
	timeout time.Duration
}

// NewOllamaModelCatalog builds a model catalog for Ollama-backed model
// discovery. baseURL may include /v1.
func NewOllamaModelCatalog(baseURL string, client *http.Client) *OllamaModelCatalog {
	if client == nil {
		client = http.DefaultClient
	}
	return &OllamaModelCatalog{
		baseURL: baseURL,
		client:  client,
		timeout: 5 * time.Second,
	}
}

// List fetches and returns sorted model names, preferring models that expose
// both completion and tools capabilities when capability metadata is present.
func (c *OllamaModelCatalog) List(ctx context.Context) ([]string, error) {
	base := strings.TrimRight(c.baseURL, "/")
	base = strings.TrimSuffix(base, "/v1")
	if base == "" {
		return nil, fmt.Errorf("LLM_BASE_URL is empty")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	if c.timeout > 0 {
		reqCtx, cancel := context.WithTimeout(req.Context(), c.timeout)
		defer cancel()
		req = req.WithContext(reqCtx)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ollama tags request failed: %s", resp.Status)
	}

	var payload struct {
		Models []struct {
			Name         string   `json:"name"`
			Capabilities []string `json:"capabilities"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(payload.Models))
	for _, m := range payload.Models {
		if m.Name == "" {
			continue
		}
		if len(m.Capabilities) > 0 {
			hasCompletion := false
			hasTools := false
			for _, cap := range m.Capabilities {
				if cap == "completion" {
					hasCompletion = true
				}
				if cap == "tools" {
					hasTools = true
				}
			}
			if !hasCompletion || !hasTools {
				continue
			}
		}
		names = append(names, m.Name)
	}
	sort.Strings(names)
	return names, nil
}
