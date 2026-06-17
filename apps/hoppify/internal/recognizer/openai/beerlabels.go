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
	"github.com/openai/openai-go/shared"
)

const (
	defaultBaseURL       = "https://api.openai.com/v1"
	defaultModel         = "chatgpt-5.4-mini"
	defaultTimeout       = 30 * time.Second
	promptVersion        = "beer-label-v1"
	openAIResponseFormat = "beer_label_recognition"
)

//go:embed prompt.md
var beerLabelPrompt string

type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
}

type Client struct {
	client openaisdk.Client
	model  string
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

	return &Client{
		client: openaisdk.NewClient(options...),
		model:  firstNonEmpty(cfg.Model, defaultModel),
	}, nil
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

	return result, nil
}

func (client *Client) Model() string {
	return client.model
}

func (client *Client) PromptVersion() string {
	return promptVersion
}

func (client *Client) newRequest(image []byte) responses.ResponseNewParams {
	imageURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image)
	imageContent := responses.ResponseInputContentParamOfInputImage(responses.ResponseInputImageDetailHigh)
	imageContent.OfInputImage.ImageURL = openaisdk.String(imageURL)

	format := responses.ResponseFormatTextConfigUnionParam{
		OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
			Name:        openAIResponseFormat,
			Description: openaisdk.String("Beer label identification result extracted only from visible image evidence."),
			Schema:      beerLabelSchema(),
			Strict:      openaisdk.Bool(true),
		},
	}

	return responses.ResponseNewParams{
		Model:             shared.ResponsesModel(client.model),
		Instructions:      openaisdk.String(strings.TrimSpace(beerLabelPrompt)),
		Store:             openaisdk.Bool(false),
		MaxOutputTokens:   openaisdk.Int(800),
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
}

func beerLabelSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required": []string{
			"status", "container", "beerName", "brewery", "style", "country", "abv", "confidence", "evidence", "notes",
		},
		"properties": map[string]any{
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
