package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func request(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	New(Options{}).ServeHTTP(rec, req)
	return rec
}

func TestHealth(t *testing.T) {
	rec := request(t, http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "ok\n" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestWebhook(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"event", `{"eventType":"Test","instanceName":"Radarr"}`, http.StatusAccepted},
		{"no event type", `{"instanceName":"Radarr"}`, http.StatusBadRequest},
		{"not json", `hello`, http.StatusBadRequest},
		{"too large", `{"eventType":"` + strings.Repeat("x", maxBody) + `"}`, http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := request(t, http.MethodPost, "/webhook", tt.body); rec.Code != tt.want {
				t.Fatalf("got %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestWebhookRejectsGet(t *testing.T) {
	if rec := request(t, http.MethodGet, "/webhook", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got %d", rec.Code)
	}
}
