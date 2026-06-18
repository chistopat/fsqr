package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	beerlabelmodel "github.com/chistopat/hoppify/internal/models/beerlabel"
)

const testResponseText = `{
	"status":"identified",
	"container":"can",
	"beerName":"Punk IPA",
	"brewery":"BrewDog",
	"style":null,
	"country":null,
	"abv":5.4,
	"confidence":0.82,
	"evidence":["label text"],
	"notes":null
}`

const testResponseTextV4 = `{
	"status":"",
	"container":"can",
	"beerName":"Punk IPA",
	"brewery":"BrewDog",
	"style":"IPA",
	"country":"Scotland",
	"abv":5.4,
	"confidence":0.9,
	"evidence":["visible Punk IPA text","grounded result confirms BrewDog Punk IPA"],
	"notes":null,
	"webSearch":{"used":false,"queries":[],"sources":[]},
	"untappd":{
		"status":"direct_match",
		"url":"https://untappd.com/b/brewdog-punk-ipa/123",
		"searchUrl":null,
		"name":"Punk IPA",
		"brewery":"BrewDog",
		"confidence":0.88,
		"reason":"Untappd result matches visible beer and brewery."
	}
}`

func TestClientUsesGenerateContentWithImageAndSchema(t *testing.T) {
	t.Parallel()

	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash-lite:generateContent" {
			t.Fatalf("expected generateContent path, got %q", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Fatalf("unexpected x-goog-api-key header: %q", r.Header.Get("x-goog-api-key"))
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"finishReason": "STOP",
				"content": map[string]any{
					"role": "model",
					"parts": []map[string]any{{
						"text": testResponseText,
					}},
				},
			}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "gemini-2.5-flash-lite",
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.IdentifyBeerLabel(context.Background(), []byte("jpeg"))
	if err != nil {
		t.Fatalf("identify beer label: %v", err)
	}

	if result.Status != beerlabelmodel.StatusIdentified || result.Container != beerlabelmodel.ContainerCan {
		t.Fatalf("unexpected result: %#v", result)
	}
	if client.PromptVersion() != PromptVersionV3 {
		t.Fatalf("expected v3 prompt version, got %q", client.PromptVersion())
	}

	assertGeminiImageRequest(t, requestBody)
	assertGeminiSchemaRequest(t, requestBody)
}

func TestClientV4UsesGoogleSearchGrounding(t *testing.T) {
	t.Parallel()

	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"finishReason": "STOP",
				"content": map[string]any{
					"role": "model",
					"parts": []map[string]any{{
						"text": "```json\n" + testResponseTextV4 + "\n```",
					}},
				},
				"groundingMetadata": map[string]any{
					"webSearchQueries": []string{"BrewDog Punk IPA Untappd"},
					"searchEntryPoint": map[string]any{"renderedContent": "<div>Search suggestions</div>"},
					"groundingChunks": []map[string]any{{
						"web": map[string]any{
							"title": "Punk IPA - BrewDog",
							"uri":   "https://untappd.com/b/brewdog-punk-ipa/123",
						},
					}},
				},
			}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		APIKey:         "test-key",
		BaseURL:        server.URL,
		Model:          "gemini-2.5-flash-lite",
		Timeout:        2 * time.Second,
		GroundedSearch: true,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.IdentifyBeerLabel(context.Background(), []byte("jpeg"))
	if err != nil {
		t.Fatalf("identify beer label: %v", err)
	}

	if client.PromptVersion() != PromptVersionV4 {
		t.Fatalf("expected v4 prompt version, got %q", client.PromptVersion())
	}
	if result.Status != beerlabelmodel.StatusIdentified {
		t.Fatalf("expected repaired identified status, got %q", result.Status)
	}
	if result.Untappd == nil || result.Untappd.Status != beerlabelmodel.UntappdDirectMatch {
		t.Fatalf("expected untappd direct match, got %#v", result.Untappd)
	}
	if result.WebSearch == nil || !result.WebSearch.Used || len(result.WebSearch.Sources) != 1 {
		t.Fatalf("expected grounded web search result, got %#v", result.WebSearch)
	}
	if result.WebSearch.SearchEntryPointHTML == nil || *result.WebSearch.SearchEntryPointHTML == "" {
		t.Fatalf("expected search entry point html, got %#v", result.WebSearch.SearchEntryPointHTML)
	}

	assertGeminiImageRequest(t, requestBody)
	assertGeminiGroundedRequest(t, requestBody)
}

func assertGeminiImageRequest(t *testing.T, requestBody map[string]any) {
	t.Helper()

	contents, ok := requestBody["contents"].([]any)
	if !ok || len(contents) != 1 {
		t.Fatalf("expected one content item, got %#v", requestBody["contents"])
	}
	content, ok := contents[0].(map[string]any)
	if !ok {
		t.Fatalf("expected content object, got %#v", contents[0])
	}
	parts, ok := content["parts"].([]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("expected text and image parts, got %#v", content["parts"])
	}
	textPart, ok := parts[0].(map[string]any)
	if !ok || textPart["text"] != identifyInstruction {
		t.Fatalf("expected identify instruction part, got %#v", parts[0])
	}
	imagePart, ok := parts[1].(map[string]any)
	if !ok {
		t.Fatalf("expected image part object, got %#v", parts[1])
	}
	inlineData, ok := imagePart["inlineData"].(map[string]any)
	if !ok {
		t.Fatalf("expected inlineData image, got %#v", imagePart)
	}
	if inlineData["mimeType"] != imageMIMEType {
		t.Fatalf("expected image/jpeg MIME type, got %#v", inlineData["mimeType"])
	}
	if inlineData["data"] != "anBlZw==" {
		t.Fatalf("expected base64 image bytes, got %#v", inlineData["data"])
	}
}

func assertGeminiSchemaRequest(t *testing.T, requestBody map[string]any) {
	t.Helper()

	config, ok := requestBody["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("expected generationConfig object, got %#v", requestBody["generationConfig"])
	}
	if config["responseMimeType"] != responseMIMEType {
		t.Fatalf("expected responseMimeType %q, got %#v", responseMIMEType, config["responseMimeType"])
	}
	if config["tools"] != nil {
		t.Fatalf("expected no tools in generation config, got %#v", config["tools"])
	}
	schema, ok := config["responseSchema"].(map[string]any)
	if !ok {
		t.Fatalf("expected responseSchema object, got %#v", config["responseSchema"])
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected schema properties, got %#v", schema["properties"])
	}
	if properties["beerName"] == nil || properties["brewery"] == nil || properties["confidence"] == nil {
		t.Fatalf("expected beer label properties, got %#v", properties)
	}
}

func assertGeminiGroundedRequest(t *testing.T, requestBody map[string]any) {
	t.Helper()

	config, ok := requestBody["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("expected generationConfig object, got %#v", requestBody["generationConfig"])
	}
	if config["responseMimeType"] != nil || config["responseSchema"] != nil {
		t.Fatalf("expected no JSON response config with grounding, got %#v", config)
	}
	tools, ok := requestBody["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("expected one google search tool, got %#v", requestBody["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("expected tool object, got %#v", tools[0])
	}
	if tool["googleSearch"] == nil && tool["google_search"] == nil {
		t.Fatalf("expected google search tool, got %#v", tool)
	}
}
