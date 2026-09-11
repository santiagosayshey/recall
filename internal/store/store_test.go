package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santiagosayshey/recall/internal/decide"
	"github.com/santiagosayshey/recall/internal/webhook"
)

var at = time.Date(2026, 9, 11, 17, 39, 29, 0, time.FixedZone("ACST", 9*3600+1800))

func parse(t *testing.T, name string) webhook.Event {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "webhook", "testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	e, err := webhook.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func grab(t *testing.T, name string) webhook.Grab {
	t.Helper()
	g, ok := parse(t, name).(*webhook.Grab)
	if !ok {
		t.Fatalf("%s is not a grab", name)
	}
	return *g
}

func imp(t *testing.T, name string) webhook.Import {
	t.Helper()
	i, ok := parse(t, name).(*webhook.Import)
	if !ok {
		t.Fatalf("%s is not an import", name)
	}
	return *i
}

func openTest(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir, Options{Now: func() time.Time { return at }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func lines(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 4<<20)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("line %d: %v", len(out)+1, err)
		}
		out = append(out, m)
	}
	return out
}

func TestLines(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir)
	if err := s.AddGrab(grab(t, "radarr-grab")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddImport(imp(t, "radarr-import")); err != nil {
		t.Fatal(err)
	}
	g0 := grab(t, "radarr-grab")
	if err := s.AddDecision(decide.Decide(&g0, imp(t, "radarr-import"))); err != nil {
		t.Fatal(err)
	}

	decisions := lines(t, filepath.Join(dir, decisionsFile))
	if len(decisions) != 1 || decisions[0]["result"] != "drift" || decisions[0]["delta"] != float64(-1180999) || decisions[0]["at"] != "2026-09-11T17:39:29+09:30" {
		t.Errorf("decision line: %v", decisions)
	}
	if _, ok := decisions[0]["raw"]; ok {
		t.Error("a decision has no raw body")
	}

	grabs := lines(t, filepath.Join(dir, grabsFile))
	if len(grabs) != 1 {
		t.Fatalf("%d grab lines", len(grabs))
	}
	g := grabs[0]
	if g["at"] != "2026-09-11T17:39:29+09:30" || g["app"] != "radarr" || g["downloadId"] != "0000000000000000000000000000000000000001" || g["score"] != float64(881400) {
		t.Errorf("grab line: %v", g)
	}
	if raw, ok := g["raw"].(map[string]any); !ok || raw["eventType"] != "Grab" {
		t.Errorf("raw body not nested as JSON: %v", g["raw"])
	}
	if _, ok := g["fileName"]; ok {
		t.Error("grab line has an import's field")
	}

	imports := lines(t, filepath.Join(dir, importsFile))
	if len(imports) != 1 {
		t.Fatalf("%d import lines", len(imports))
	}
	i := imports[0]
	if i["fileName"] != "100 Percent Wolf.2020.1080p.BluRay.DD5.1.x264- PTer" || i["score"] != float64(-299599) || i["isUpgrade"] != false {
		t.Errorf("import line: %v", i)
	}
	if _, ok := i["group"]; ok {
		t.Error("empty group should be omitted")
	}

	// The raw body is the last key, so the record reads first.
	raw, _ := os.ReadFile(filepath.Join(dir, grabsFile))
	if !strings.HasSuffix(strings.TrimSpace(string(raw)), "}}") || !strings.HasPrefix(string(raw), `{"at":`) {
		t.Errorf("line order: %.60s ... %.20s", raw, raw[len(raw)-20:])
	}
}

func TestGrabsSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	s := openTest(t, dir)
	if err := s.AddGrab(grab(t, "sonarr-grab-pack")); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s = openTest(t, dir)
	g, ok := s.Grab("0000000000000000000000000000000000000004")
	if !ok || g.Score != 923690 || g.Media.Title != "Sherlock" || len(g.Media.Episodes) != 3 {
		t.Fatalf("after restart: %v %+v", ok, g)
	}
	if !json.Valid(g.Raw) || len(g.Raw) < 1000 {
		t.Errorf("raw body not restored: %d bytes", len(g.Raw))
	}
	if s.Grabs() != 1 {
		t.Errorf("%d grabs", s.Grabs())
	}
}

func TestLaterGrabReplaces(t *testing.T) {
	s := openTest(t, t.TempDir())
	first := grab(t, "radarr-grab")
	second := first
	second.Score = 1
	second.ReleaseTitle = "second"
	for _, g := range []webhook.Grab{first, second} {
		if err := s.AddGrab(g); err != nil {
			t.Fatal(err)
		}
	}
	if g, _ := s.Grab(first.DownloadID); g.ReleaseTitle != "second" || s.Grabs() != 1 {
		t.Errorf("got %q, %d grabs", g.ReleaseTitle, s.Grabs())
	}
}

func TestUnknownDownload(t *testing.T) {
	s := openTest(t, t.TempDir())
	if _, ok := s.Grab("nope"); ok {
		t.Error("found a grab that was never added")
	}
}

func TestOpenErrors(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "missing"), Options{}); err == nil {
		t.Error("opened a directory that does not exist")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, grabsFile), []byte("{not json}\n"), 0o644)
	if _, err := Open(dir, Options{}); err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Errorf("corrupt grab file: %v", err)
	}
	for _, name := range []string{importsFile, decisionsFile} {
		dir = t.TempDir()
		os.Mkdir(filepath.Join(dir, name), 0o755)
		if _, err := Open(dir, Options{}); err == nil {
			t.Errorf("opened with a directory where %s goes", name)
		}
	}
}

func TestAppendAfterClose(t *testing.T) {
	s := openTest(t, t.TempDir())
	s.Close()
	if err := s.AddGrab(grab(t, "radarr-grab")); err == nil {
		t.Error("appended to a closed store")
	}
	if err := s.AddImport(imp(t, "radarr-import")); err == nil {
		t.Error("appended to a closed store")
	}
	if err := s.AddDecision(decide.Decision{}); err == nil {
		t.Error("appended to a closed store")
	}
}
