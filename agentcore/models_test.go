package agentcore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaModelCatalogListFiltersAndSorts(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Fatalf("path: got %q want %q", r.URL.Path, "/api/tags")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"zeta","capabilities":["completion","tools"]},{"name":"alpha","capabilities":["completion","tools"]},{"name":"no-tools","capabilities":["completion"]}]}`))
	}))
	defer ts.Close()

	catalog := NewOllamaModelCatalog(ts.URL+"/v1", ts.Client())
	models, err := catalog.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"alpha", "zeta"}
	if len(models) != len(want) {
		t.Fatalf("got %v want %v", models, want)
	}
	for i := range want {
		if models[i] != want[i] {
			t.Fatalf("got %v want %v", models, want)
		}
	}
}

func TestOllamaModelCatalogListStatusError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadGateway)
	}))
	defer ts.Close()

	catalog := NewOllamaModelCatalog(ts.URL, ts.Client())
	_, err := catalog.List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ollama tags request failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOllamaModelCatalogListEmptyBaseURL(t *testing.T) {
	catalog := NewOllamaModelCatalog("", http.DefaultClient)
	_, err := catalog.List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "LLM_BASE_URL is empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}
