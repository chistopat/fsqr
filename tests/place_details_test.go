//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

func TestE2E_PLACE_DETAILS_001(t *testing.T) {
	env := newTestEnv(t)
	env.loadFixtures(t, cleanupFixture, categoriesCoreFixture, placeDetailsFixture)

	body := env.getJSON(t, "/api/v1/places/tc-place-details-full", http.StatusOK)
	assertPlaceDetailsShape(t, body)

	var place placeDetailsResponse
	decodeJSON(t, body, &place)
	if place.UUID != "tc-place-details-full" {
		t.Fatalf("expected uuid tc-place-details-full, got %q: %s", place.UUID, body)
	}
	if place.Category == nil || place.Category.FSQCategoryID != "4bf58dd8d48988d1e0931735" {
		t.Fatalf("expected coffee category, got %#v: %s", place.Category, body)
	}
	if place.Address == nil || place.Address.Line == nil || *place.Address.Line == "" {
		t.Fatalf("expected address to be present: %s", body)
	}
	if place.Contacts == nil || place.Contacts.Tel == nil || *place.Contacts.Tel == "" {
		t.Fatalf("expected contacts to be present: %s", body)
	}
}

func TestE2E_PLACE_DETAILS_002(t *testing.T) {
	env := newTestEnv(t)
	env.loadFixtures(t, cleanupFixture, categoriesCoreFixture, placeDetailsFixture)

	body := env.getJSON(t, "/api/v1/places/tc-place-details-minimal", http.StatusOK)
	assertPlaceDetailsShape(t, body)

	var place placeDetailsResponse
	decodeJSON(t, body, &place)
	if place.UUID != "tc-place-details-minimal" {
		t.Fatalf("expected uuid tc-place-details-minimal, got %q: %s", place.UUID, body)
	}
	if place.Name == "" || place.Lat == 0 || place.Lon == 0 {
		t.Fatalf("expected required fields to be present: %s", body)
	}
	if place.Address != nil {
		t.Fatalf("expected address to be omitted, got %#v: %s", place.Address, body)
	}
	if place.Contacts != nil {
		t.Fatalf("expected contacts to be omitted, got %#v: %s", place.Contacts, body)
	}
}

func TestE2E_PLACE_404_001(t *testing.T) {
	env := newTestEnv(t)
	env.loadFixtures(t, cleanupFixture, categoriesCoreFixture, placeDetailsFixture)

	body := env.getJSON(t, "/api/v1/places/tc-place-missing", http.StatusNotFound)
	assertErrorResponse(t, body, "not_found", "place was not found")
}
