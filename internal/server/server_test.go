package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
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
	log   *bytes.Buffer
}

func newHarness(t *testing.T) harness {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	var log bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&log, &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}}))
	return harness{New(Options{Store: st, Logger: logger}), dir, st, &log}
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
	if got := h.lines(t, "decisions.jsonl"); got != 3 {
		t.Errorf("%d decision lines, want 3", got)
	}
	raw, _ := os.ReadFile(filepath.Join(h.dir, "decisions.jsonl"))
	var results []string
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		var d struct{ Result string }
		json.Unmarshal(line, &d)
		results = append(results, d.Result)
	}
	if want := "drift clean clean"; strings.Join(results, " ") != want {
		t.Errorf("decisions %v, want %s", results, want)
	}

	want := `level=INFO msg=decision result=drift instance=Radarr media="100% Wolf (2020)" release="100 Percent Wolf 2020 1080p BluRay DD5.1 x264-PTer" file="100 Percent Wolf.2020.1080p.BluRay.DD5.1.x264- PTer" grabScore=881400 importScore=-299599 delta=-1180999 lost="[1080p Quality Tier 5]" gained="[Release Group (Missing)]" path="/media/test-library/100% Wolf (2020) {tmdb-520946}/100% Wolf (2020) {tmdb-520946} [Bluray-1080p][AC3 5.1][x264].mkv" download=0000000000000000000000000000000000000001`
	if !strings.Contains(h.log.String(), want+"\n") {
		t.Errorf("drift line not logged as expected; log:\n%s", h.log.String())
	}
}

func TestWebhookImportWithoutGrabIsUnmatched(t *testing.T) {
	h := newHarness(t)
	if rec := h.request(http.MethodPost, "/webhook", fixture(t, "sonarr-import-episode")); rec.Code != http.StatusAccepted {
		t.Fatalf("got %d", rec.Code)
	}
	if !strings.Contains(h.log.String(), `msg=decision result=unmatched instance=Sonarr media="Sherlock (2010) S02E01"`) {
		t.Errorf("log:\n%s", h.log.String())
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
	for _, name := range []string{"radarr-grab", "radarr-import"} {
		if rec := h.request(http.MethodPost, "/webhook", fixture(t, name)); rec.Code != http.StatusInternalServerError {
			t.Fatalf("%s: got %d", name, rec.Code)
		}
	}
}
