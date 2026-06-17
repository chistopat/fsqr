package openai

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	beerlabelmodel "github.com/chistopat/hoppify/internal/models/beerlabel"

	openaisdk "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
)

const (
	defaultBaseURL       = "https://api.openai.com/v1"
	defaultModel         = "gpt-5.4-mini"
	defaultTimeout       = 30 * time.Second
	openAIResponseFormat = "beer_label_recognition"

	PromptVersionV1 = "beer-label-v1"
	PromptVersionV2 = "beer-label-v2-web"
)

//go:embed prompt.md
var beerLabelPromptV1 string

//go:embed prompt_v2.md
var beerLabelPromptV2 string

type Config struct {
	APIKey    string
	BaseURL   string
	Model     string
	Timeout   time.Duration
	WebSearch bool
}

type Client struct {
	client        openaisdk.Client
	model         string
	prompt        string
	promptVersion string
	webSearch     bool
}

func NewClient(cfg Config) (*Client, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("openai api key is required")
	}

	options := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithHTTPClient(&http.Client{Timeout: normalizeTimeout(cfg.Timeout)}),
	}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		options = append(options, option.WithBaseURL(strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")))
	} else {
		options = append(options, option.WithBaseURL(defaultBaseURL))
	}

	client := &Client{
		client:        openaisdk.NewClient(options...),
		model:         firstNonEmpty(cfg.Model, defaultModel),
		prompt:        strings.TrimSpace(beerLabelPromptV1),
		promptVersion: PromptVersionV1,
		webSearch:     cfg.WebSearch,
	}
	if cfg.WebSearch {
		client.prompt = strings.TrimSpace(beerLabelPromptV2)
		client.promptVersion = PromptVersionV2
	}

	return client, nil
}

func (client *Client) IdentifyBeerLabel(ctx context.Context, image []byte) (beerlabelmodel.Result, error) {
	response, err := client.client.Responses.New(ctx, client.newRequest(image))
	if err != nil {
		return beerlabelmodel.Result{}, fmt.Errorf("openai responses request: %w", err)
	}
	if response.Status != "" && response.Status != responses.ResponseStatusCompleted {
		return beerlabelmodel.Result{}, fmt.Errorf("openai response status %q", response.Status)
	}

	text := strings.TrimSpace(response.OutputText())
	if text == "" {
		return beerlabelmodel.Result{}, fmt.Errorf("openai response did not contain output text")
	}

	var result beerlabelmodel.Result
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return beerlabelmodel.Result{}, fmt.Errorf("decode beer label result: %w", err)
	}
	client.enrichWebResult(&result, response)

	return result, nil
}

func (client *Client) Model() string {
	return client.model
}

func (client *Client) PromptVersion() string {
	return client.promptVersion
}

func (client *Client) newRequest(image []byte) responses.ResponseNewParams {
	imageURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image)
	imageContent := responses.ResponseInputContentParamOfInputImage(responses.ResponseInputImageDetailHigh)
	imageContent.OfInputImage.ImageURL = openaisdk.String(imageURL)

	formatDescription := "Beer label identification result extracted only from visible image evidence."
	if client.webSearch {
		formatDescription = "Beer label identification result with web verification and Untappd recommendation."
	}
	format := responses.ResponseFormatTextConfigUnionParam{
		OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
			Name:        openAIResponseFormat,
			Description: openaisdk.String(formatDescription),
			Schema:      beerLabelSchema(client.webSearch),
			Strict:      openaisdk.Bool(true),
		},
	}

	params := responses.ResponseNewParams{
		Model:             client.model,
		Instructions:      openaisdk.String(client.prompt),
		Store:             openaisdk.Bool(false),
		MaxOutputTokens:   openaisdk.Int(client.maxOutputTokens()),
		Temperature:       openaisdk.Float(0),
		ParallelToolCalls: openaisdk.Bool(false),
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam{
				responses.ResponseInputItemParamOfMessage(
					responses.ResponseInputMessageContentListParam{
						responses.ResponseInputContentParamOfInputText(
							"Identify the beer container and label shown in this image.",
						),
						imageContent,
					},
					responses.EasyInputMessageRoleUser,
				),
			},
		},
		Text: responses.ResponseTextConfigParam{Format: format},
	}
	if client.webSearch {
		params.MaxToolCalls = openaisdk.Int(4)
		params.Tools = []responses.ToolUnionParam{webSearchTool()}
	}

	return params
}

func (client *Client) maxOutputTokens() int64 {
	if client.webSearch {
		return 1600
	}

	return 800
}

func webSearchTool() responses.ToolUnionParam {
	tool := responses.ToolParamOfWebSearchPreview(responses.WebSearchToolType("web_search"))
	tool.OfWebSearchPreview.SearchContextSize = responses.WebSearchToolSearchContextSizeLow

	return tool
}

func (client *Client) enrichWebResult(result *beerlabelmodel.Result, response *responses.Response) {
	if !client.webSearch {
		return
	}
	if result.WebSearch == nil {
		result.WebSearch = &beerlabelmodel.WebSearchResult{}
	}
	for itemIndex := range response.Output {
		item := &response.Output[itemIndex]
		if item.Type == "web_search_call" {
			result.WebSearch.Used = true
			action := item.AsWebSearchCall().Action
			switch action.Type {
			case "search":
				result.WebSearch.Queries = appendTrimmedString(result.WebSearch.Queries, action.Query)
			case "open_page":
				result.WebSearch.Sources = appendWebSource(result.WebSearch.Sources, "", action.URL)
			}
		}
		for contentIndex := range item.Content {
			content := &item.Content[contentIndex]
			for annotationIndex := range content.Annotations {
				annotation := &content.Annotations[annotationIndex]
				if annotation.Type == "url_citation" {
					result.WebSearch.Sources = appendWebSource(
						result.WebSearch.Sources,
						annotation.Title,
						annotation.URL,
					)
				}
			}
		}
	}
	result.WebSearch.Queries = uniqueStrings(result.WebSearch.Queries)
	result.WebSearch.Sources = uniqueSources(result.WebSearch.Sources)
}

func beerLabelSchema(webSearch bool) map[string]any {
	required := []string{
		"status", "container", "beerName", "brewery", "style", "country", "abv", "confidence", "evidence", "notes",
	}
	properties := map[string]any{
		"status": map[string]any{
			"type": "string",
			"enum": []string{
				beerlabelmodel.StatusIdentified,
				beerlabelmodel.StatusUncertain,
				beerlabelmodel.StatusUnreadable,
				beerlabelmodel.StatusNotBeer,
			},
		},
		"container": map[string]any{
			"type": "string",
			"enum": []string{
				beerlabelmodel.ContainerBottle,
				beerlabelmodel.ContainerCan,
				beerlabelmodel.ContainerGlass,
				beerlabelmodel.ContainerOther,
				beerlabelmodel.ContainerUnknown,
			},
		},
		"beerName":   nullableStringSchema(),
		"brewery":    nullableStringSchema(),
		"style":      nullableStringSchema(),
		"country":    nullableStringSchema(),
		"abv":        nullableNumberSchema(0, 100),
		"confidence": numberSchema(0, 1),
		"evidence": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
		"notes": nullableStringSchema(),
	}
	if webSearch {
		required = append(required, "webSearch", "untappd")
		properties["webSearch"] = webSearchSchema()
		properties["untappd"] = untappdSchema()
	}

	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           properties,
	}
}

func webSearchSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"used", "queries", "sources"},
		"properties": map[string]any{
			"used": map[string]any{"type": "boolean"},
			"queries": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"sources": map[string]any{
				"type":  "array",
				"items": webSourceSchema(),
			},
		},
	}
}

func webSourceSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"title", "url"},
		"properties": map[string]any{
			"title": nullableStringSchema(),
			"url":   map[string]any{"type": "string"},
		},
	}
}

func untappdSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"status", "url", "searchUrl", "name", "brewery", "confidence", "reason"},
		"properties": map[string]any{
			"status": map[string]any{
				"type": "string",
				"enum": []string{
					beerlabelmodel.UntappdDirectMatch,
					beerlabelmodel.UntappdSearchRecommended,
					beerlabelmodel.UntappdAmbiguous,
					beerlabelmodel.UntappdNotFound,
					beerlabelmodel.UntappdNotApplicable,
				},
			},
			"url":        nullableStringSchema(),
			"searchUrl":  nullableStringSchema(),
			"name":       nullableStringSchema(),
			"brewery":    nullableStringSchema(),
			"confidence": numberSchema(0, 1),
			"reason":     nullableStringSchema(),
		},
	}
}

func nullableStringSchema() map[string]any {
	return map[string]any{"type": []string{"string", "null"}}
}

func nullableNumberSchema(minimum, maximum float64) map[string]any {
	return map[string]any{
		"type":    []string{"number", "null"},
		"minimum": minimum,
		"maximum": maximum,
	}
}

func numberSchema(minimum, maximum float64) map[string]any {
	return map[string]any{
		"type":    "number",
		"minimum": minimum,
		"maximum": maximum,
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

func appendTrimmedString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}

	return append(values, value)
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
			trimmed := strings.TrimSpace(*source.Title)
			source.Title = optionalString(trimmed)
		}
		if _, ok := seen[source.URL]; ok {
			continue
		}
		seen[source.URL] = struct{}{}
		unique = append(unique, source)
	}

	return unique
}
