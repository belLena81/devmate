package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"devmate/internal/infra/llm"
)

// newTestClient creates an OllamaClient pointed at the given test server URL.
// Model is fixed to "test-model" for all unit tests.
func newTestClient(serverURL string) *llm.OllamaClient {
	return llm.NewOllamaClient(
		llm.WithBaseURL(serverURL),
		llm.WithModel("test-model"),
	)
}

// ollamaResponse builds a minimal valid Ollama /api/generate response body.
func ollamaResponse(response string) string {
	b, _ := json.Marshal(map[string]any{
		"model":    "test-model",
		"response": response,
		"done":     true,
	})
	return string(b)
}

// ─── Construction ────────────────────────────────────────────────────────────

func TestNewOllamaClient_DefaultURL(t *testing.T) {
	c := llm.NewOllamaClient()
	if c == nil {
		t.Fatal("NewOllamaClient returned nil")
	}
}

func TestNewOllamaClient_DefaultModel_IsSet(t *testing.T) {
	c := llm.NewOllamaClient()
	if c.Model() == "" {
		t.Error("expected a non-empty default model")
	}
}

func TestNewOllamaClient_WithModel_OverridesDefault(t *testing.T) {
	c := llm.NewOllamaClient(llm.WithModel("llama3.2"))
	if c.Model() != "llama3.2" {
		t.Errorf("expected model %q, got %q", "llama3.2", c.Model())
	}
}

func TestNewOllamaClient_WithBaseURL_OverridesDefault(t *testing.T) {
	c := llm.NewOllamaClient(llm.WithBaseURL("http://remotehost:11434"))
	if c.BaseURL() != "http://remotehost:11434" {
		t.Errorf("expected base URL %q, got %q", "http://remotehost:11434", c.BaseURL())
	}
}

// ─── Happy path ──────────────────────────────────────────────────────────────

func TestGenerate_ReturnsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ollamaResponse("feat: add auth")))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.Generate(context.Background(), "write a commit message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "feat: add auth" {
		t.Errorf("expected %q, got %q", "feat: add auth", got)
	}
}

func TestGenerate_TrimsWhitespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ollamaResponse("  feat: add auth\n\n")))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	got, err := c.Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "feat: add auth" {
		t.Errorf("expected trimmed response, got %q", got)
	}
}

// ─── Request shape ───────────────────────────────────────────────────────────

func TestGenerate_SendsPOSTToGenerateEndpoint(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Write([]byte(ollamaResponse("ok")))
	}))
	defer srv.Close()

	newTestClient(srv.URL).Generate(context.Background(), "prompt")

	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/api/generate" {
		t.Errorf("expected path /api/generate, got %s", gotPath)
	}
}

func TestGenerate_SendsPromptInBody(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.Write([]byte(ollamaResponse("ok")))
	}))
	defer srv.Close()

	newTestClient(srv.URL).Generate(context.Background(), "my special prompt")

	if body["prompt"] != "my special prompt" {
		t.Errorf("expected prompt in body, got %v", body["prompt"])
	}
}

func TestGenerate_SendsModelInBody(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.Write([]byte(ollamaResponse("ok")))
	}))
	defer srv.Close()

	newTestClient(srv.URL).Generate(context.Background(), "prompt")

	if body["model"] != "test-model" {
		t.Errorf("expected model %q in body, got %v", "test-model", body["model"])
	}
}

func TestGenerate_DisablesStreaming(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.Write([]byte(ollamaResponse("ok")))
	}))
	defer srv.Close()

	newTestClient(srv.URL).Generate(context.Background(), "prompt")

	stream, ok := body["stream"].(bool)
	if !ok || stream {
		t.Errorf("expected stream=false in body, got %v", body["stream"])
	}
}

func TestGenerate_SetsJSONContentType(t *testing.T) {
	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.Write([]byte(ollamaResponse("ok")))
	}))
	defer srv.Close()

	newTestClient(srv.URL).Generate(context.Background(), "prompt")

	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Errorf("expected application/json Content-Type, got %q", gotContentType)
	}
}

// ─── Error handling ──────────────────────────────────────────────────────────

func TestGenerate_ServerUnavailable_ReturnsError(t *testing.T) {
	c := llm.NewOllamaClient(llm.WithBaseURL("http://127.0.0.1:1")) // nothing listening
	_, err := c.Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error when server is unavailable")
	}
}

func TestGenerate_Non200Status_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error on 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to mention status code, got: %v", err)
	}
}

func TestGenerate_500Status_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestGenerate_MalformedJSON_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Generate(context.Background(), "prompt")
	if err == nil {
		t.Fatal("expected error on malformed JSON response")
	}
}

func TestGenerate_EmptyResponseField_ReturnsEmptyString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(ollamaResponse("")))
	}))
	defer srv.Close()

	got, err := newTestClient(srv.URL).Generate(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
