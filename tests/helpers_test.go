//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	defaultBaseURL         = "http://127.0.0.1:3000"
	defaultTestDatabaseURL = "postgres://fsqr:fsqr@127.0.0.1:5432/fsqr_test?sslmode=disable"

	cleanupFixture        = "docs/test-cases/fixtures/cleanup.sql"
	categoriesCoreFixture = "docs/test-cases/fixtures/categories-core.sql"
	coffeePaphosFixture   = "docs/test-cases/fixtures/search-coffee-paphos.sql"
	fuelPaphosFixture     = "docs/test-cases/fixtures/search-fuel-paphos.sql"
	highLatitudeFixture   = "docs/test-cases/fixtures/search-high-latitude.sql"
	geographyFixture      = "docs/test-cases/fixtures/search-geography-paphos.sql"
	antimeridianFixture   = "docs/test-cases/fixtures/search-antimeridian.sql"
	placeDetailsFixture   = "docs/test-cases/fixtures/place-details.sql"
)

type testEnv struct {
	root    string
	db      *sql.DB
	baseURL string
}

type validationCase struct {
	path            string
	messageContains string
}

type categoryCase struct {
	path          string
	wantLength    int
	wantFirstID   int64
	wantFirstFSQ  string
	wantFirstName string
}

type searchCase struct {
	fixture   string
	path      string
	wantUUIDs []string
	excluded  []string
}

type errorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type categoryResponse struct {
	ID            int64  `json:"id"`
	FSQCategoryID string `json:"fsq_category_id"`
	Name          string `json:"name"`
	Label         string `json:"label"`
	Level         int    `json:"level"`
}

type searchResponse struct {
	TookMS int64         `json:"took_ms"`
	Places []searchPlace `json:"places"`
}

type searchPlace struct {
	UUID string  `json:"uuid"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

type placeDetailsResponse struct {
	UUID     string         `json:"uuid"`
	Name     string         `json:"name"`
	Lat      float64        `json:"lat"`
	Lon      float64        `json:"lon"`
	Category *placeCategory `json:"category,omitempty"`
	Address  *placeAddress  `json:"address,omitempty"`
	Contacts *placeContacts `json:"contacts,omitempty"`
}

type placeCategory struct {
	FSQCategoryID string `json:"fsq_category_id"`
	Name          string `json:"name"`
	Path          string `json:"path"`
}

type placeAddress struct {
	Line     *string `json:"line,omitempty"`
	Locality *string `json:"locality,omitempty"`
	Region   *string `json:"region,omitempty"`
	Country  *string `json:"country,omitempty"`
}

type placeContacts struct {
	Tel        *string `json:"tel,omitempty"`
	Website    *string `json:"website,omitempty"`
	Email      *string `json:"email,omitempty"`
	FacebookID *int64  `json:"facebook_id,omitempty"`
	Instagram  *string `json:"instagram,omitempty"`
	Twitter    *string `json:"twitter,omitempty"`
}

func runValidationCase(t *testing.T, tc validationCase) {
	t.Helper()

	env := newTestEnv(t)
	env.loadFixtures(t, cleanupFixture)

	body := env.getJSON(t, tc.path, http.StatusBadRequest)
	assertErrorResponse(t, body, "invalid_request", tc.messageContains)
}

func runCategoryCase(t *testing.T, tc categoryCase) {
	t.Helper()

	env := newTestEnv(t)
	env.loadFixtures(t, cleanupFixture, categoriesCoreFixture)

	body := env.getJSON(t, tc.path, http.StatusOK)
	assertCategoryShape(t, body)

	var categories []categoryResponse
	decodeJSON(t, body, &categories)
	if len(categories) == 0 {
		t.Fatalf("expected at least one category, got body: %s", body)
	}
	if tc.wantLength > 0 && len(categories) != tc.wantLength {
		t.Fatalf("expected %d categories, got %d: %s", tc.wantLength, len(categories), body)
	}
	if categories[0].ID != tc.wantFirstID {
		t.Fatalf("expected first category id %d, got %d: %s", tc.wantFirstID, categories[0].ID, body)
	}
	if tc.wantFirstFSQ != "" && categories[0].FSQCategoryID != tc.wantFirstFSQ {
		t.Fatalf("expected first category fsq id %q, got %q: %s", tc.wantFirstFSQ, categories[0].FSQCategoryID, body)
	}
	if categories[0].Name != tc.wantFirstName {
		t.Fatalf("expected first category name %q, got %q: %s", tc.wantFirstName, categories[0].Name, body)
	}
}

func runSearchCase(t *testing.T, tc searchCase) {
	t.Helper()

	env := newTestEnv(t)
	env.loadFixtures(t, cleanupFixture, categoriesCoreFixture, tc.fixture)

	body := env.getJSON(t, tc.path, http.StatusOK)
	response := assertSearchResponse(t, body)
	got := placeUUIDs(response.Places)
	if !reflect.DeepEqual(got, tc.wantUUIDs) {
		t.Fatalf("unexpected place order:\nwant: %v\n got: %v\nbody: %s", tc.wantUUIDs, got, body)
	}
	assertExcludedUUIDs(t, got, tc.excluded)
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	db := openDB(t, ctx)
	env := &testEnv{
		root:    repoRoot(t),
		db:      db,
		baseURL: envDefault("BASE_URL", defaultBaseURL),
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()

		if err := execSQLFiles(cleanupCtx, db, env.root, cleanupFixture); err != nil {
			t.Logf("cleanup test database: %v", err)
		}
		_ = db.Close()
	})

	return env
}

func (env *testEnv) loadFixtures(t *testing.T, fixtures ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if err := execSQLFiles(ctx, env.db, env.root, fixtures...); err != nil {
		t.Fatalf("load fixtures: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()

		if err := execSQLFiles(cleanupCtx, env.db, env.root, cleanupFixture); err != nil {
			t.Logf("cleanup test database: %v", err)
		}
	})
}

func openDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()

	db, err := sql.Open("pgx", envDefault("TEST_DATABASE_URL", defaultTestDatabaseURL))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("ping test database: %v", err)
	}

	return db
}

func execSQLFiles(ctx context.Context, db *sql.DB, root string, names ...string) error {
	for _, name := range names {
		path := filepath.Join(root, name)
		query, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := db.ExecContext(ctx, string(query)); err != nil {
			return fmt.Errorf("exec %s: %w", name, err)
		}
	}

	return nil
}

func (env *testEnv) getJSON(t *testing.T, path string, expectedStatus int) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, env.baseURL+path, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if response.StatusCode != expectedStatus {
		t.Fatalf("expected status %d, got %d\nbody: %s", expectedStatus, response.StatusCode, body)
	}
	assertJSONContentType(t, response.Header.Get("Content-Type"))

	return body
}

func assertJSONContentType(t *testing.T, contentType string) {
	t.Helper()

	if !strings.Contains(contentType, "application/json") {
		t.Fatalf("expected JSON content type, got %q", contentType)
	}
}

func assertErrorResponse(t *testing.T, body []byte, expectedCode string, expectedMessage string) {
	t.Helper()
	assertObjectKeys(t, body, []string{"error"})

	var raw map[string]map[string]any
	decodeJSON(t, body, &raw)
	assertMapKeys(t, raw["error"], []string{"code", "message"})

	var response errorResponse
	decodeJSON(t, body, &response)
	if response.Error.Code != expectedCode {
		t.Fatalf("expected error code %q, got %q: %s", expectedCode, response.Error.Code, body)
	}
	if !strings.Contains(response.Error.Message, expectedMessage) {
		t.Fatalf("expected error message to contain %q, got %q: %s", expectedMessage, response.Error.Message, body)
	}
}

func assertCategoryShape(t *testing.T, body []byte) {
	t.Helper()

	var categories []map[string]any
	decodeJSON(t, body, &categories)
	for _, category := range categories {
		assertMapKeys(t, category, []string{"id", "fsq_category_id", "name", "label", "level"})
	}
}

func assertSearchResponse(t *testing.T, body []byte) searchResponse {
	t.Helper()
	assertObjectKeys(t, body, []string{"took_ms", "places"})

	var raw struct {
		Places []map[string]any `json:"places"`
	}
	decodeJSON(t, body, &raw)
	if raw.Places == nil {
		t.Fatalf("expected places to be an array: %s", body)
	}
	for _, place := range raw.Places {
		assertMapKeys(t, place, []string{"uuid", "name", "lat", "lon"})
	}

	var response searchResponse
	decodeJSON(t, body, &response)
	if response.TookMS < 0 {
		t.Fatalf("expected non-negative took_ms, got %d: %s", response.TookMS, body)
	}

	return response
}

func assertPlaceDetailsShape(t *testing.T, body []byte) {
	t.Helper()

	var place map[string]any
	decodeJSON(t, body, &place)
	assertMapKeys(t, place, []string{"uuid", "name", "lat", "lon", "category", "address", "contacts"})

	if rawCategory, ok := place["category"]; ok {
		category, ok := rawCategory.(map[string]any)
		if !ok {
			t.Fatalf("expected category object, got %#v: %s", rawCategory, body)
		}
		assertMapKeys(t, category, []string{"fsq_category_id", "name", "path"})
	}
	if rawAddress, ok := place["address"]; ok {
		address, ok := rawAddress.(map[string]any)
		if !ok {
			t.Fatalf("expected address object, got %#v: %s", rawAddress, body)
		}
		assertMapKeys(t, address, []string{"line", "locality", "region", "country"})
	}
	if rawContacts, ok := place["contacts"]; ok {
		contacts, ok := rawContacts.(map[string]any)
		if !ok {
			t.Fatalf("expected contacts object, got %#v: %s", rawContacts, body)
		}
		assertMapKeys(t, contacts, []string{"tel", "website", "email", "facebook_id", "instagram", "twitter"})
	}
}

func assertObjectKeys(t *testing.T, body []byte, allowed []string) {
	t.Helper()

	var object map[string]any
	decodeJSON(t, body, &object)
	assertMapKeys(t, object, allowed)
}

func assertMapKeys(t *testing.T, object map[string]any, allowed []string) {
	t.Helper()

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}
	for key := range object {
		if _, ok := allowedSet[key]; !ok {
			t.Fatalf("unexpected JSON field %q in object %#v", key, object)
		}
	}
}

func placeUUIDs(places []searchPlace) []string {
	uuids := make([]string, 0, len(places))
	for _, place := range places {
		uuids = append(uuids, place.UUID)
	}

	return uuids
}

func assertExcludedUUIDs(t *testing.T, actual []string, excluded []string) {
	t.Helper()

	seen := make(map[string]struct{}, len(actual))
	for _, uuid := range actual {
		seen[uuid] = struct{}{}
	}
	for _, uuid := range excluded {
		if _, ok := seen[uuid]; ok {
			t.Fatalf("did not expect uuid %q in response: %v", uuid, actual)
		}
	}
}

func decodeJSON(t *testing.T, body []byte, out any) {
	t.Helper()

	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode response: %v\nbody: %s", err, body)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root")
		}
		dir = parent
	}
}

func envDefault(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}

	return value
}
