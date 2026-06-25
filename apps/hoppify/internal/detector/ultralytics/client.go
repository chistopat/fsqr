package ultralytics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	detectionmodel "github.com/chistopat/hoppify/internal/models/detection"
)

const (
	defaultTimeout       = 30 * time.Second
	defaultImageSize     = 640
	defaultConfidence    = 0.25
	defaultIOU           = 0.7
	defaultJPEGQuality   = 95
	defaultMaxDetections = 300
	maxErrorBodyBytes    = 4096
)

type Config struct {
	EndpointURL         string
	APIKey              string
	Timeout             time.Duration
	ImageSize           int
	ConfidenceThreshold float64
	IOUThreshold        float64
	MaxDetections       int
	JPEGQuality         int
}

type Client struct {
	endpointURL         string
	apiKey              string
	httpClient          *http.Client
	imageSize           int
	confidenceThreshold float64
	iouThreshold        float64
	maxDetections       int
	jpegQuality         int
}

type predictResponse struct {
	Images   []predictImage `json:"images"`
	Metadata map[string]any `json:"metadata"`
}

type predictImage struct {
	Shape   []int                `json:"shape"`
	Results []predictResult      `json:"results"`
	Speed   detectionmodel.Speed `json:"speed"`
}

type predictResult struct {
	Class      int                 `json:"class"`
	Name       string              `json:"name"`
	Confidence float64             `json:"confidence"`
	Box        *detectionmodel.Box `json:"box"`
	OBB        json.RawMessage     `json:"obb"`
	Polygon    json.RawMessage     `json:"polygon"`
	Points     json.RawMessage     `json:"points"`
	XYXYXYXY   json.RawMessage     `json:"xyxyxyxy"`
	XYWHR      json.RawMessage     `json:"xywhr"`
}

func NewClient(cfg Config) (*Client, error) {
	cfg = normalizeConfig(cfg)
	if strings.TrimSpace(cfg.EndpointURL) == "" {
		return nil, fmt.Errorf("ultralytics endpoint url is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("ultralytics api key is required")
	}

	return &Client{
		endpointURL:         strings.TrimSpace(cfg.EndpointURL),
		apiKey:              strings.TrimSpace(cfg.APIKey),
		httpClient:          &http.Client{Timeout: cfg.Timeout},
		imageSize:           cfg.ImageSize,
		confidenceThreshold: cfg.ConfidenceThreshold,
		iouThreshold:        cfg.IOUThreshold,
		maxDetections:       cfg.MaxDetections,
		jpegQuality:         cfg.JPEGQuality,
	}, nil
}

func (client *Client) Detect(
	ctx context.Context,
	img image.Image,
) (detectionmodel.ImageResult, error) {
	response, err := client.predict(ctx, img)
	if err != nil {
		return detectionmodel.ImageResult{}, err
	}
	if len(response.Images) == 0 {
		return detectionmodel.ImageResult{Shape: imageShape(img)}, nil
	}

	imageResult := response.Images[0].imageResult()
	if imageResult.Shape == [2]int{} {
		imageResult.Shape = imageShape(img)
	}
	if client.maxDetections > 0 && len(imageResult.Results) > client.maxDetections {
		imageResult.Results = imageResult.Results[:client.maxDetections]
	}

	return imageResult, nil
}

func (client *Client) predict(ctx context.Context, img image.Image) (predictResponse, error) {
	body, contentType, err := client.multipartBody(img)
	if err != nil {
		return predictResponse{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpointURL, body)
	if err != nil {
		return predictResponse{}, fmt.Errorf("build ultralytics request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.apiKey)
	request.Header.Set("Content-Type", contentType)

	response, err := client.httpClient.Do(request)
	if err != nil {
		return predictResponse{}, fmt.Errorf("call ultralytics predict endpoint: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		errorBody, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes))
		return predictResponse{}, fmt.Errorf(
			"ultralytics predict endpoint returned %d: %s",
			response.StatusCode,
			strings.TrimSpace(string(errorBody)),
		)
	}

	var prediction predictResponse
	if err := json.NewDecoder(response.Body).Decode(&prediction); err != nil {
		return predictResponse{}, fmt.Errorf("decode ultralytics response: %w", err)
	}

	return prediction, nil
}

func (client *Client) multipartBody(img image.Image) (*bytes.Buffer, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	fields := map[string]string{
		"conf":  strconv.FormatFloat(client.confidenceThreshold, 'f', -1, 64),
		"iou":   strconv.FormatFloat(client.iouThreshold, 'f', -1, 64),
		"imgsz": strconv.Itoa(client.imageSize),
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, "", fmt.Errorf("write ultralytics field %s: %w", key, err)
		}
	}

	fileWriter, err := writer.CreateFormFile("file", "image.jpg")
	if err != nil {
		return nil, "", fmt.Errorf("create ultralytics image field: %w", err)
	}
	if err := jpeg.Encode(fileWriter, img, &jpeg.Options{Quality: client.jpegQuality}); err != nil {
		return nil, "", fmt.Errorf("encode ultralytics image jpeg: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("close ultralytics multipart body: %w", err)
	}

	return &body, writer.FormDataContentType(), nil
}

func (img predictImage) imageResult() detectionmodel.ImageResult {
	shape := shapeFromSlice(img.Shape)
	detections := make([]detectionmodel.Detection, 0, len(img.Results))
	for index := range img.Results {
		detection, ok := img.Results[index].detection(shape)
		if ok {
			detections = append(detections, detection)
		}
	}

	return detectionmodel.ImageResult{
		Shape:   shape,
		Results: detections,
		Speed:   img.Speed,
	}
}

func (result *predictResult) detection(shape [2]int) (detectionmodel.Detection, bool) {
	box, ok := result.detectionBox(shape)
	if !ok {
		return detectionmodel.Detection{}, false
	}

	return detectionmodel.Detection{
		Class:      result.Class,
		Name:       result.Name,
		Confidence: round5(result.Confidence),
		Box:        box,
	}, true
}

func (result *predictResult) detectionBox(shape [2]int) (detectionmodel.Box, bool) {
	if result.Box != nil {
		box := scaleNormalizedBox(*result.Box, shape)
		box = clampBox(box, shape)
		if box.X2 > box.X1 && box.Y2 > box.Y1 {
			return roundBox(box), true
		}
	}

	points, _, ok := result.points(shape)
	if !ok {
		return detectionmodel.Box{}, false
	}

	box := boxFromPoints(points)
	box = clampBox(box, shape)
	if box.X2 <= box.X1 || box.Y2 <= box.Y1 {
		return detectionmodel.Box{}, false
	}

	return roundBox(box), true
}

func shapeFromSlice(shape []int) [2]int {
	if len(shape) < 2 {
		return [2]int{}
	}

	return [2]int{shape[0], shape[1]}
}

func imageShape(img image.Image) [2]int {
	bounds := img.Bounds()

	return [2]int{bounds.Dy(), bounds.Dx()}
}

func normalizeConfig(cfg Config) Config {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	if cfg.ImageSize <= 0 {
		cfg.ImageSize = defaultImageSize
	}
	if cfg.ConfidenceThreshold <= 0 {
		cfg.ConfidenceThreshold = defaultConfidence
	}
	if cfg.IOUThreshold <= 0 {
		cfg.IOUThreshold = defaultIOU
	}
	if cfg.MaxDetections <= 0 {
		cfg.MaxDetections = defaultMaxDetections
	}
	if cfg.JPEGQuality <= 0 || cfg.JPEGQuality > 100 {
		cfg.JPEGQuality = defaultJPEGQuality
	}

	return cfg
}
