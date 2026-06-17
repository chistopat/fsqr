package embeddings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	embeddingmodel "github.com/chistopat/fsqr/internal/models/embedding"
	querymodel "github.com/chistopat/fsqr/internal/models/search/query"
)

func TestOpenAIClientEmbedQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", request.Method)
		}
		if request.URL.Path != "/v1/embeddings" {
			t.Fatalf("expected /v1/embeddings path, got %s", request.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body["model"] != "test-model" {
			t.Fatalf("expected model test-model, got %#v", body["model"])
		}
		if body["input"] != "query: coffee" {
			t.Fatalf("expected query-prefixed input, got %#v", body["input"])
		}

		values := make([]float64, embeddingmodel.Dimensions)
		for index := range values {
			values[index] = float64(index) / 1000
		}

		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(map[string]any{
			"object": "list",
			"model":  "test-model",
			"data": []map[string]any{
				{
					"object":    "embedding",
					"index":     0,
					"embedding": values,
				},
			},
			"usage": map[string]any{
				"prompt_tokens": 1,
				"total_tokens":  1,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	_, err := NewOpenAIClient(OpenAIConfig{
		BaseURL: server.URL + "/v1",
		APIKey:  "test-key",
		Model:   "test-model",
		Timeout: 0,
	})
	if err == nil {
		t.Fatal("expected missing timeout to fail")
	}

	client, err := NewOpenAIClient(OpenAIConfig{
		BaseURL: server.URL + "/v1",
		APIKey:  "test-key",
		Model:   "test-model",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new openai client: %v", err)
	}

	query, err := querymodel.New("coffee", querymodel.DefaultLimit)
	if err != nil {
		t.Fatal(err)
	}

	embedding, err := client.EmbedQuery(t.Context(), query)
	if err != nil {
		t.Fatalf("embed query: %v", err)
	}
	if embedding.Len() != embeddingmodel.Dimensions {
		t.Fatalf("expected %d dimensions, got %d", embeddingmodel.Dimensions, embedding.Len())
	}
}
