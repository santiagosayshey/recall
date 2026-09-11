package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santiagosayshey/recall/internal/store"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "webhook", "testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type harness struct {
	*Server
	dir   string
	store *store.Store
}

func newHarness(t *testing.T) harness {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return harness{New(Options{Store: st}), dir, st}
}

func (h harness) request(method, path string, body []byte) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, bytes.NewReader(body)))
	return rec
}

func (h harness) lines(t *testing.T, file string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(h.dir, file))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(raw), "\n")
}

func TestHealth(t *testing.T) {
	rec := newHarness(t).request(http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "ok\n" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestWebhookStoresGrabsAndImports(t *testing.T) {
	h := newHarness(t)
	for _, name := range []string{"radarr-test", "radarr-grab", "radarr-import", "sonarr-grab-pack", "sonarr-import-pack-e01", "sonarr-import-pack-e02", "sonarr-import-pack-summary", "sonarr-health"} {
		if rec := h.request(http.MethodPost, "/webhook", fixture(t, name)); rec.Code != http.StatusAccepted {
			t.Fatalf("%s: got %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if got := h.lines(t, "grabs.jsonl"); got != 2 {
		t.Errorf("%d grab lines, want 2", got)
	}
	if got := h.lines(t, "imports.jsonl"); got != 3 {
		t.Errorf("%d import lines, want 3", got)
	}
	if g, ok := h.store.Grab("0000000000000000000000000000000000000001"); !ok || g.Score != 881400 {
		t.Errorf("radarr grab not found: %v %d", ok, g.Score)
	}
}

func TestWebhookRejects(t *testing.T) {
	h := newHarness(t)
	tests := []struct {
		name string
		body string
		want int
	}{
		{"no event type", `{"instanceName":"Radarr"}`, http.StatusBadRequest},
		{"not json", `hello`, http.StatusBadRequest},
		{"too large", `{"eventType":"` + strings.Repeat("x", maxBody) + `"}`, http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := h.request(http.MethodPost, "/webhook", []byte(tt.body)); rec.Code != tt.want {
				t.Fatalf("got %d, want %d", rec.Code, tt.want)
			}
		})
	}
	if rec := h.request(http.MethodGet, "/webhook", nil); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: got %d", rec.Code)
	}
}

func TestWebhookStoreFailure(t *testing.T) {
	h := newHarness(t)
	h.store.Close() // every append now fails
	if rec := h.request(http.MethodPost, "/webhook", fixture(t, "radarr-grab")); rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d", rec.Code)
	}
}
