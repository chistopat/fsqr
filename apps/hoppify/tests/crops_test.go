//go:build e2e

package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	_ "image/jpeg"
	"io"
	"net/http"
	"strings"
	"testing"
)

type cropsResponse struct {
	UUID  string     `json:"uuid"`
	Crops []cropItem `json:"crops"`
}

type cropItem struct {
	UUID string `json:"uuid"`
	Type string `json:"type"`
	URI  string `json:"uri"`
}

func TestCreateCropsStoresImageCropInS3AndPostgres(t *testing.T) {
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

	parent := response.Captures[0]
	parentRow := queryCapture(t, db, parent.UUID)
	t.Cleanup(func() {
		deleteObject(t, s3Client, parentRow.Bucket, parentRow.ObjectKey)
	})

	crops := postCrops(t, baseURL, parent.UUID, [][]float64{{1, 1, 3, 4}})
	if len(crops.Crops) != 1 {
		t.Fatalf("expected one crop, got %d", len(crops.Crops))
	}

	crop := crops.Crops[0]
	cropRow := queryCapture(t, db, crop.UUID)
	t.Cleanup(func() {
		deleteCapture(t, db, parent.UUID)
		deleteObject(t, s3Client, cropRow.Bucket, cropRow.ObjectKey)
	})

	if crop.Type != "image_crop" {
		t.Fatalf("expected image_crop type, got %q", crop.Type)
	}
	assertParentUUID(t, db, crop.UUID, parent.UUID)
	if !strings.HasPrefix(cropRow.ObjectKey, "captures/crops/"+parent.UUID+"/") {
		t.Fatalf("expected crop object under parent prefix, got %q", cropRow.ObjectKey)
	}
	if !strings.HasSuffix(cropRow.ObjectKey, crop.UUID+".jpg") {
		t.Fatalf("expected crop object to use crop uuid filename, got %q", cropRow.ObjectKey)
	}
	if strings.Contains(cropRow.ObjectKey, "_000") {
		t.Fatalf("expected crop object key without numbered suffix, got %q", cropRow.ObjectKey)
	}

	body := getObjectBody(t, s3Client, cropRow.Bucket, cropRow.ObjectKey)
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("decode crop object: %v", err)
	}
	if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 3 {
		t.Fatalf("expected crop dimensions 2x3, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}

	retry := postCrops(t, baseURL, parent.UUID, [][]float64{{1, 1, 3, 4}})
	if len(retry.Crops) != 1 || retry.Crops[0] != crop {
		t.Fatalf("expected retry to return same crop, got %#v", retry.Crops)
	}
	assertChildCaptureCount(t, db, parent.UUID, 1)
}

func postCrops(t *testing.T, baseURL string, captureUUID string, boxes [][]float64) cropsResponse {
	t.Helper()

	requestBoxes := make([]map[string]any, 0, len(boxes))
	for index := range boxes {
		requestBoxes = append(requestBoxes, map[string]any{"bbox": boxes[index]})
	}
	requestBody, err := json.Marshal(map[string]any{
		"uuid":  captureUUID,
		"boxes": requestBoxes,
	})
	if err != nil {
		t.Fatalf("marshal crops request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		baseURL+"/api/v1/crops",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		t.Fatalf("build crops request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("post crops: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read crops response: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("expected status %d, got %d with body %q", http.StatusCreated, response.StatusCode, string(body))
	}

	var crops cropsResponse
	if err := json.Unmarshal(body, &crops); err != nil {
		t.Fatalf("decode crops response: %v", err)
	}

	return crops
}

func assertParentUUID(t *testing.T, db *sql.DB, captureUUID string, parentUUID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), databaseQueryTimeout)
	defer cancel()

	var actual sql.NullString
	if err := db.QueryRowContext(
		ctx,
		"SELECT parent_uuid::text FROM captures WHERE uuid = $1",
		captureUUID,
	).Scan(&actual); err != nil {
		t.Fatalf("query parent uuid: %v", err)
	}
	if !actual.Valid || actual.String != parentUUID {
		t.Fatalf("expected parent uuid %q, got %#v", parentUUID, actual)
	}
}

func assertChildCaptureCount(t *testing.T, db *sql.DB, parentUUID string, expected int64) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), databaseQueryTimeout)
	defer cancel()

	var count int64
	if err := db.QueryRowContext(
		ctx,
		"SELECT count(*) FROM captures WHERE parent_uuid = $1",
		parentUUID,
	).Scan(&count); err != nil {
		t.Fatalf("query child capture count: %v", err)
	}
	if count != expected {
		t.Fatalf("expected %d child captures, got %d", expected, count)
	}
}
