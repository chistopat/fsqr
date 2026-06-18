package gemini

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	beerlabelmodel "github.com/chistopat/hoppify/internal/models/beerlabel"

	"google.golang.org/genai"
)

const (
	defaultModel        = "gemini-2.5-flash-lite"
	defaultTimeout      = 30 * time.Second
	responseMIMEType    = "application/json"
	maxOutputTokens     = 800
	maxGroundedTokens   = 1800
	imageMIMEType       = "image/jpeg"
	PromptVersionV3     = "beer-label-v3-gemini-2.5-flash-lite"
	PromptVersionV4     = "beer-label-v4-gemini-grounded-search"
	identifyInstruction = "Identify the beer container and label shown in this image."
)

//go:embed prompt.md
var beerLabelPromptV3 string

//go:embed prompt_v4.md
var beerLabelPromptV4 string

type Config struct {
	APIKey         string
	BaseURL        string
	Model          string
	Timeout        time.Duration
	GroundedSearch bool
}

type Client struct {
	client         *genai.Client
	model          string
	prompt         string
	promptVersion  string
	timeout        time.Duration
	groundedSearch bool
}

func NewClient(cfg Config) (*Client, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("gemini api key is required")
	}

	timeout := normalizeTimeout(cfg.Timeout)
	clientConfig := &genai.ClientConfig{
		APIKey:     apiKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: &http.Client{Timeout: timeout},
		HTTPOptions: genai.HTTPOptions{
			Timeout: &timeout,
		},
	}
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		clientConfig.HTTPOptions.BaseURL = strings.TrimRight(baseURL, "/") + "/"
	}

	sdkClient, err := genai.NewClient(context.Background(), clientConfig)
	if err != nil {
		return nil, fmt.Errorf("create gemini client: %w", err)
	}

	client := &Client{
		client:        sdkClient,
		model:         firstNonEmpty(cfg.Model, defaultModel),
		prompt:        strings.TrimSpace(beerLabelPromptV3),
		promptVersion: PromptVersionV3,
		timeout:       timeout,
	}
	if cfg.GroundedSearch {
		client.prompt = strings.TrimSpace(beerLabelPromptV4)
		client.promptVersion = PromptVersionV4
		client.groundedSearch = true
	}

	return client, nil
}

func (client *Client) IdentifyBeerLabel(ctx context.Context, image []byte) (beerlabelmodel.Result, error) {
	requestCtx, cancel := context.WithTimeout(ctx, client.timeout)
	defer cancel()

	response, err := client.client.Models.GenerateContent(
		requestCtx,
		client.model,
		client.newContents(image),
		client.newConfig(),
	)
	if err != nil {
		return beerlabelmodel.Result{}, fmt.Errorf("gemini generate content request: %w", err)
	}

	text := strings.TrimSpace(response.Text())
	if text == "" {
		return beerlabelmodel.Result{}, fmt.Errorf("gemini response did not contain output text")
	}

	result, err := decodeBeerLabelResult(text)
	if err != nil {
		return beerlabelmodel.Result{}, fmt.Errorf("decode beer label result: %w", err)
	}
	client.enrichGroundingResult(&result, response)
	client.repairGroundedResult(&result)

	return result, nil
}

func (client *Client) Model() string {
	return client.model
}

func (client *Client) PromptVersion() string {
	return client.promptVersion
}

func (client *Client) newContents(image []byte) []*genai.Content {
	return []*genai.Content{
		genai.NewContentFromParts(
			[]*genai.Part{
				genai.NewPartFromText(identifyInstruction),
				genai.NewPartFromBytes(image, imageMIMEType),
			},
			genai.RoleUser,
		),
	}
}

func (client *Client) newConfig() *genai.GenerateContentConfig {
	temperature := float32(0)

	cfg := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{genai.NewPartFromText(client.prompt)},
		},
		Temperature:     &temperature,
		MaxOutputTokens: maxOutputTokens,
	}
	if client.groundedSearch {
		cfg.MaxOutputTokens = maxGroundedTokens
		cfg.Tools = []*genai.Tool{{GoogleSearch: &genai.GoogleSearch{}}}

		return cfg
	}

	cfg.ResponseMIMEType = responseMIMEType
	cfg.ResponseSchema = beerLabelSchema()

	return cfg
}

func beerLabelSchema() *genai.Schema {
	ordering := []string{
		"status", "container", "beerName", "brewery", "style", "country", "abv", "confidence", "evidence", "notes",
	}

	return &genai.Schema{
		Type:             genai.TypeObject,
		Description:      "Beer label identification result extracted only from visible image evidence.",
		Required:         ordering,
		PropertyOrdering: ordering,
		Properties: map[string]*genai.Schema{
			"status": enumStringSchema(
				"Identification status.",
				beerlabelmodel.StatusIdentified,
				beerlabelmodel.StatusUncertain,
				beerlabelmodel.StatusUnreadable,
				beerlabelmodel.StatusNotBeer,
			),
			"container": enumStringSchema(
				"Visible container type.",
				beerlabelmodel.ContainerBottle,
				beerlabelmodel.ContainerCan,
				beerlabelmodel.ContainerGlass,
				beerlabelmodel.ContainerOther,
				beerlabelmodel.ContainerUnknown,
			),
			"beerName":   nullableStringSchema("Beer name directly readable in the image, otherwise null."),
			"brewery":    nullableStringSchema("Brewery directly readable in the image, otherwise null."),
			"style":      nullableStringSchema("Beer style directly readable in the image, otherwise null."),
			"country":    nullableStringSchema("Country directly readable in the image, otherwise null."),
			"abv":        nullableNumberSchema("ABV percentage directly readable in the image, otherwise null.", 0, 100),
			"confidence": numberSchema("Confidence from 0 to 1 based on visible evidence strength.", 0, 1),
			"evidence": {
				Type:        genai.TypeArray,
				Description: "Short visible evidence snippets or visual cues. No guesses.",
				Items:       &genai.Schema{Type: genai.TypeString},
			},
			"notes": nullableStringSchema("Brief limitation when useful, otherwise null."),
		},
	}
}

func enumStringSchema(description string, values ...string) *genai.Schema {
	return &genai.Schema{
		Type:        genai.TypeString,
		Format:      "enum",
		Description: description,
		Enum:        values,
	}
}

func nullableStringSchema(description string) *genai.Schema {
	return &genai.Schema{
		Type:        genai.TypeString,
		Description: description,
		Nullable:    boolPtr(true),
	}
}

func nullableNumberSchema(description string, minimum, maximum float64) *genai.Schema {
	return &genai.Schema{
		Type:        genai.TypeNumber,
		Format:      "double",
		Description: description,
		Nullable:    boolPtr(true),
		Minimum:     floatPtr(minimum),
		Maximum:     floatPtr(maximum),
	}
}

func numberSchema(description string, minimum, maximum float64) *genai.Schema {
	return &genai.Schema{
		Type:        genai.TypeNumber,
		Format:      "double",
		Description: description,
		Minimum:     floatPtr(minimum),
		Maximum:     floatPtr(maximum),
	}
}

func decodeBeerLabelResult(text string) (beerlabelmodel.Result, error) {
	var result beerlabelmodel.Result
	if err := json.Unmarshal([]byte(text), &result); err == nil {
		return result, nil
	}

	candidate := extractJSONObject(text)
	if candidate == "" || candidate == text {
		return beerlabelmodel.Result{}, fmt.Errorf("invalid json object")
	}
	if err := json.Unmarshal([]byte(candidate), &result); err != nil {
		return beerlabelmodel.Result{}, fmt.Errorf("parse extracted json object: %w", err)
	}

	return result, nil
}

func extractJSONObject(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	start := strings.Index(text, "{")
	if start < 0 {
		return ""
	}

	depth := 0
	inString := false
	escaped := false
	for index := start; index < len(text); index++ {
		character := text[index]
		if inString {
			switch {
			case escaped:
				escaped = false
			case character == '\\':
				escaped = true
			case character == '"':
				inString = false
			}
			continue
		}

		switch character {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(text[start : index+1])
			}
		}
	}

	return ""
}

func (client *Client) enrichGroundingResult(result *beerlabelmodel.Result, response *genai.GenerateContentResponse) {
	if !client.groundedSearch {
		return
	}
	if result.WebSearch == nil {
		result.WebSearch = &beerlabelmodel.WebSearchResult{}
	}
	for _, candidate := range response.Candidates {
		if candidate == nil || candidate.GroundingMetadata == nil {
			continue
		}
		metadata := candidate.GroundingMetadata
		result.WebSearch.Queries = append(result.WebSearch.Queries, metadata.WebSearchQueries...)
		result.WebSearch.Queries = append(result.WebSearch.Queries, metadata.ImageSearchQueries...)
		if metadata.SearchEntryPoint != nil {
			result.WebSearch.Used = true
			result.WebSearch.SearchEntryPointHTML = optionalString(metadata.SearchEntryPoint.RenderedContent)
		}
		for _, chunk := range metadata.GroundingChunks {
			if chunk == nil || chunk.Web == nil {
				continue
			}
			result.WebSearch.Sources = appendWebSource(result.WebSearch.Sources, chunk.Web.Title, chunk.Web.URI)
		}
	}
	result.WebSearch.Queries = uniqueStrings(result.WebSearch.Queries)
	result.WebSearch.Sources = uniqueSources(result.WebSearch.Sources)
	if len(result.WebSearch.Queries) > 0 || len(result.WebSearch.Sources) > 0 {
		result.WebSearch.Used = true
	}
}

func (client *Client) repairGroundedResult(result *beerlabelmodel.Result) {
	if !client.groundedSearch || strings.TrimSpace(result.Status) != "" {
		return
	}
	switch {
	case result.Untappd != nil &&
		result.Untappd.Status == beerlabelmodel.UntappdDirectMatch &&
		result.BeerName != nil &&
		result.Brewery != nil:
		result.Status = beerlabelmodel.StatusIdentified
	case result.BeerName != nil || result.Brewery != nil:
		result.Status = beerlabelmodel.StatusUncertain
	default:
		result.Status = beerlabelmodel.StatusUnreadable
	}
}

func normalizeTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return defaultTimeout
	}

	return timeout
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

func appendWebSource(sources []beerlabelmodel.WebSource, title, rawURL string) []beerlabelmodel.WebSource {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return sources
	}

	return append(sources, beerlabelmodel.WebSource{
		Title: optionalString(title),
		URL:   rawURL,
	})
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	return &value
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}

	return unique
}

func uniqueSources(sources []beerlabelmodel.WebSource) []beerlabelmodel.WebSource {
	seen := make(map[string]struct{}, len(sources))
	unique := make([]beerlabelmodel.WebSource, 0, len(sources))
	for _, source := range sources {
		source.URL = strings.TrimSpace(source.URL)
		if source.URL == "" {
			continue
		}
		if source.Title != nil {
			source.Title = optionalString(*source.Title)
		}
		if _, ok := seen[source.URL]; ok {
			continue
		}
		seen[source.URL] = struct{}{}
		unique = append(unique, source)
	}

	return unique
}

func boolPtr(value bool) *bool {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}
