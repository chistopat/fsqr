package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	beerlabelmodel "github.com/chistopat/hoppify/internal/models/beerlabel"
)

const testResponseText = `{
	"status":"identified",
	"container":"bottle",
	"beerName":"Punk IPA",
	"brewery":"BrewDog",
	"style":null,
	"country":null,
	"abv":5.4,
	"confidence":0.82,
	"evidence":["label text"],
	"notes":null
}`

func TestClientUsesResponsesAPIWithImageAndJSONSchema(t *testing.T) {
	t.Parallel()

	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("expected /responses path, got %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"id":         "resp_test",
			"object":     "response",
			"created_at": 1,
			"model":      "chatgpt-5.4-mini",
			"status":     "completed",
			"output": []map[string]any{{
				"id":     "msg_test",
				"type":   "message",
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]any{{
					"type":        "output_text",
					"text":        testResponseText,
					"annotations": []any{},
				}},
			}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "chatgpt-5.4-mini",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.IdentifyBeerLabel(context.Background(), []byte("jpeg"))
	if err != nil {
		t.Fatalf("identify beer label: %v", err)
	}

	if result.Status != beerlabelmodel.StatusIdentified || result.Container != beerlabelmodel.ContainerBottle {
		t.Fatalf("unexpected result: %#v", result)
	}
	if requestBody["model"] != "chatgpt-5.4-mini" {
		t.Fatalf("unexpected model: %#v", requestBody["model"])
	}
	if requestBody["tools"] != nil {
		t.Fatalf("expected no tools in request, got %#v", requestBody["tools"])
	}
	if requestBody["store"] != false {
		t.Fatalf("expected store=false, got %#v", requestBody["store"])
	}

	assertJSONSchemaRequest(t, requestBody)
	assertImageRequest(t, requestBody)
}

func assertJSONSchemaRequest(t *testing.T, requestBody map[string]any) {
	t.Helper()

	text, ok := requestBody["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected text object, got %#v", requestBody["text"])
	}
	format, ok := text["format"].(map[string]any)
	if !ok {
		t.Fatalf("expected text.format object, got %#v", text["format"])
	}
	if format["type"] != "json_schema" {
		t.Fatalf("expected json_schema format, got %#v", format["type"])
	}
	if format["name"] != openAIResponseFormat {
		t.Fatalf("expected response format name %q, got %#v", openAIResponseFormat, format["name"])
	}
	if format["strict"] != true {
		t.Fatalf("expected strict json schema, got %#v", format["strict"])
	}
}

func assertImageRequest(t *testing.T, requestBody map[string]any) {
	t.Helper()

	input, ok := requestBody["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("expected one input message, got %#v", requestBody["input"])
	}
	message, ok := input[0].(map[string]any)
	if !ok {
		t.Fatalf("expected input message object, got %#v", input[0])
	}
	content, ok := message["content"].([]any)
	if !ok || len(content) != 2 {
		t.Fatalf("expected text and image content, got %#v", message["content"])
	}
	image, ok := content[1].(map[string]any)
	if !ok {
		t.Fatalf("expected image content object, got %#v", content[1])
	}
	if image["type"] != "input_image" {
		t.Fatalf("expected input_image content, got %#v", image["type"])
	}
	imageURL, ok := image["image_url"].(string)
	if !ok || !strings.HasPrefix(imageURL, "data:image/jpeg;base64,") {
		t.Fatalf("expected jpeg data URL, got %#v", image["image_url"])
	}
}
