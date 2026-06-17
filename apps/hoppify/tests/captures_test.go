//go:build e2e

package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	defaultBaseURL       = "http://127.0.0.1:3100"
	defaultMetricsURL    = "http://127.0.0.1:3101/metrics"
	defaultDatabaseURL   = "postgres://hoppify:hoppify@127.0.0.1:5432/hoppify_test?sslmode=disable"
	defaultS3Endpoint    = "http://127.0.0.1:9000"
	defaultS3Region      = "us-east-1"
	defaultS3AccessKey   = "minioadmin"
	defaultS3SecretKey   = "minioadmin"
	requestTimeout       = 10 * time.Second
	databaseQueryTimeout = 5 * time.Second
)

type captureResponse struct {
	Captures []captureItem `json:"captures"`
}

type captureItem struct {
	UUID string `json:"uuid"`
	Type string `json:"type"`
	URI  string `json:"uri"`
}

type errorResponse struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

type captureRow struct {
	Bucket         string
	ObjectKey      string
	ContentType    string
	SizeBytes      int64
	ChecksumSHA256 string
}

type multipartFile struct {
	Filename    string
	ContentType string
	Body        []byte
}

func TestCreateCaptureStoresJPEGInS3AndPostgres(t *testing.T) {
	db := openDatabase(t)
	truncateCaptures(t, db)
	s3Client := newS3Client(t)
	baseURL := envDefault("BASE_URL", defaultBaseURL)

	response := postCapture(t, baseURL, []multipartFile{{
		Filename:    "capture.png",
		ContentType: "image/png",
		Body:        newPNG(t),
	}})

	if len(response.Captures) != 1 {
		t.Fatalf("expected one capture, got %d", len(response.Captures))
	}

	capture := response.Captures[0]
	if capture.Type != "image" {
		t.Fatalf("expected image type, got %q", capture.Type)
	}

	row := queryCapture(t, db, capture.UUID)
	t.Cleanup(func() {
		deleteCapture(t, db, capture.UUID)
		deleteObject(t, s3Client, row.Bucket, row.ObjectKey)
	})

	expectedURI := fmt.Sprintf("s3://%s/%s", row.Bucket, row.ObjectKey)
	if capture.URI != expectedURI {
		t.Fatalf("expected uri %q, got %q", expectedURI, capture.URI)
	}
	if row.ContentType != "image/jpeg" {
		t.Fatalf("expected stored content type image/jpeg, got %q", row.ContentType)
	}
	if row.SizeBytes <= 0 {
		t.Fatalf("expected positive jpeg size, got %d", row.SizeBytes)
	}

	head := headObject(t, s3Client, row.Bucket, row.ObjectKey)
	if aws.ToString(head.ContentType) != "image/jpeg" {
		t.Fatalf("expected s3 content type image/jpeg, got %q", aws.ToString(head.ContentType))
	}
	if aws.ToInt64(head.ContentLength) != row.SizeBytes {
		t.Fatalf("expected s3 size %d, got %d", row.SizeBytes, aws.ToInt64(head.ContentLength))
	}
	if head.Metadata["checksum-sha256"] != row.ChecksumSHA256 {
		t.Fatalf("expected s3 checksum metadata %q, got %q", row.ChecksumSHA256, head.Metadata["checksum-sha256"])
	}

	body := getObjectBody(t, s3Client, row.Bucket, row.ObjectKey)
	checksum := sha256.Sum256(body)
	if hex.EncodeToString(checksum[:]) != row.ChecksumSHA256 {
		t.Fatalf("expected object checksum %q, got %q", row.ChecksumSHA256, hex.EncodeToString(checksum[:]))
	}

	metrics := getText(t, envDefault("HOPPIFY_METRICS_URL", defaultMetricsURL))
	if !strings.Contains(metrics, "hoppify_captures_images_total 1") {
		t.Fatalf("expected metrics to report one capture, got:\n%s", metrics)
	}
	if !strings.Contains(metrics, fmt.Sprintf("hoppify_captures_image_size_bytes_total %d", row.SizeBytes)) {
		t.Fatalf("expected metrics to report capture bytes %d, got:\n%s", row.SizeBytes, metrics)
	}
}

func TestCreateCaptureValidationRejectsBadRequests(t *testing.T) {
	db := openDatabase(t)
	truncateCaptures(t, db)
	baseURL := envDefault("BASE_URL", defaultBaseURL)

	cases := []struct {
		name       string
		files      []multipartFile
		status     int
		errorCode  string
		rawRequest bool
	}{
		{
			name:      "empty batch",
			files:     nil,
			status:    http.StatusBadRequest,
			errorCode: "invalid_request",
		},
		{
			name: "unsupported media type",
			files: []multipartFile{{
				Filename:    "capture.gif",
				ContentType: "image/gif",
				Body:        []byte("GIF89a"),
			}},
			status:    http.StatusUnsupportedMediaType,
			errorCode: "unsupported_media_type",
		},
		{
			name: "undecodable jpeg",
			files: []multipartFile{{
				Filename:    "broken.jpg",
				ContentType: "image/jpeg",
				Body:        []byte("not a jpeg"),
			}},
			status:    http.StatusBadRequest,
			errorCode: "invalid_request",
		},
		{
			name:       "broken multipart",
			status:     http.StatusBadRequest,
			errorCode:  "invalid_request",
			rawRequest: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.rawRequest {
				assertRawPostError(t, baseURL, tc.status, tc.errorCode)
				assertCaptureCount(t, db, 0)
				return
			}

			assertPostError(t, baseURL, tc.files, tc.status, tc.errorCode)
			assertCaptureCount(t, db, 0)
		})
	}
}

func postCapture(t *testing.T, baseURL string, files []multipartFile) captureResponse {
	t.Helper()

	status, body := doMultipartPost(t, baseURL+"/api/v1/captures", files)
	if status != http.StatusCreated {
		t.Fatalf("expected status %d, got %d with body %q", http.StatusCreated, status, string(body))
	}

	var response captureResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode capture response: %v", err)
	}

	return response
}

func assertPostError(t *testing.T, baseURL string, files []multipartFile, status int, code string) {
	t.Helper()

	actualStatus, body := doMultipartPost(t, baseURL+"/api/v1/captures", files)
	assertErrorResponse(t, actualStatus, body, status, code)
}

func assertRawPostError(t *testing.T, baseURL string, status int, code string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/captures", strings.NewReader("bad"))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", "multipart/form-data; boundary=missing")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("post raw multipart: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	assertErrorResponse(t, response.StatusCode, body, status, code)
}

func assertErrorResponse(t *testing.T, actualStatus int, body []byte, expectedStatus int, code string) {
	t.Helper()

	if actualStatus != expectedStatus {
		t.Fatalf("expected status %d, got %d with body %q", expectedStatus, actualStatus, string(body))
	}

	var response errorResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Error.Code != code {
		t.Fatalf("expected error code %q, got %q", code, response.Error.Code)
	}
}

func doMultipartPost(t *testing.T, url string, files []multipartFile) (int, []byte) {
	t.Helper()

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	for _, file := range files {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="files"; filename="%s"`, file.Filename))
		header.Set("Content-Type", file.ContentType)

		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatalf("create multipart part: %v", err)
		}
		if _, err := part.Write(file.Body); err != nil {
			t.Fatalf("write multipart part: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &requestBody)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("post multipart: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	return response.StatusCode, body
}

func openDatabase(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("pgx", envDefault("TEST_DATABASE_URL", defaultDatabaseURL))
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), databaseQueryTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}

	return db
}

func truncateCaptures(t *testing.T, db *sql.DB) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), databaseQueryTimeout)
	defer cancel()
	if _, err := db.ExecContext(ctx, "DELETE FROM captures"); err != nil {
		t.Fatalf("delete captures: %v", err)
	}
}

func queryCapture(t *testing.T, db *sql.DB, captureUUID string) captureRow {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), databaseQueryTimeout)
	defer cancel()

	var row captureRow
	err := db.QueryRowContext(
		ctx,
		`SELECT bucket, object_key, content_type, size_bytes, checksum_sha256
		FROM captures
		WHERE uuid = $1`,
		captureUUID,
	).Scan(&row.Bucket, &row.ObjectKey, &row.ContentType, &row.SizeBytes, &row.ChecksumSHA256)
	if err != nil {
		t.Fatalf("query capture: %v", err)
	}

	return row
}

func deleteCapture(t *testing.T, db *sql.DB, captureUUID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), databaseQueryTimeout)
	defer cancel()
	if _, err := db.ExecContext(ctx, "DELETE FROM captures WHERE uuid = $1", captureUUID); err != nil {
		t.Fatalf("delete capture: %v", err)
	}
}

func assertCaptureCount(t *testing.T, db *sql.DB, expected int64) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), databaseQueryTimeout)
	defer cancel()

	var count int64
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM captures").Scan(&count); err != nil {
		t.Fatalf("query capture count: %v", err)
	}
	if count != expected {
		t.Fatalf("expected %d captures, got %d", expected, count)
	}
}

func newS3Client(t *testing.T) *s3.Client {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(envDefault("HOPPIFY_S3_REGION", defaultS3Region)),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			envDefault("HOPPIFY_S3_ACCESS_KEY_ID", defaultS3AccessKey),
			envDefault("HOPPIFY_S3_SECRET_ACCESS_KEY", defaultS3SecretKey),
			"",
		)),
	)
	if err != nil {
		t.Fatalf("load aws config: %v", err)
	}

	endpoint := envDefault("HOPPIFY_S3_ENDPOINT_URL", defaultS3Endpoint)
	return s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
}

func headObject(t *testing.T, client *s3.Client, bucket, objectKey string) *s3.HeadObjectOutput {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	response, err := client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		t.Fatalf("head object: %v", err)
	}

	return response
}

func getObjectBody(t *testing.T, client *s3.Client, bucket, objectKey string) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	response, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read object body: %v", err)
	}

	return body
}

func deleteObject(t *testing.T, client *s3.Client, bucket, objectKey string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	if _, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	}); err != nil {
		t.Fatalf("delete object: %v", err)
	}
}

func newPNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.Set(x, y, color.RGBA{R: uint8(x * 40), G: uint8(y * 40), B: 180, A: 255})
		}
	}

	var body bytes.Buffer
	if err := png.Encode(&body, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	return body.Bytes()
}

func getText(t *testing.T, url string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("get text: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read text response: %v", err)
	}

	return string(body)
}

func envDefault(name, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}

	return value
}
