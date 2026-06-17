package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandlerServesPlaceholderPage(t *testing.T) {
	t.Parallel()

	handler := NewHandler(time.Now())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if !strings.Contains(response.Body.String(), "Hoppify") {
		t.Fatalf("expected placeholder page to mention Hoppify, got %q", response.Body.String())
	}
}

func TestHandlerServesLiveResponse(t *testing.T) {
	t.Parallel()

	handler := NewHandler(time.Now())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/live", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if strings.TrimSpace(response.Body.String()) != `{"status":"ok","service":"hoppify"}` {
		t.Fatalf("unexpected live response: %q", response.Body.String())
	}
}
