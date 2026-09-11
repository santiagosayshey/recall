//go:build integration

// Package test replays the captured webhooks through a built container
// image and checks the exact lines it logs and the files it leaves. Run it
// with `make check-integration`; CI runs it against the image the build job
// produced.
package test

import (
	"bufio"
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// image is the container image under test.
var image = os.Getenv("RECALL_IMAGE")

func TestMain(m *testing.M) {
	if image == "" {
		panic("RECALL_IMAGE must name the image to test")
	}
	os.Exit(m.Run())
}

type instance struct {
	t    *testing.T
	url  string
	data string
	cmd  *exec.Cmd
	out  *bufio.Scanner
	seen []string
}

// start runs the artifact on a free port with the given data directory and
// waits for its first log line.
func start(t *testing.T, data string) *instance {
	t.Helper()
	name := "recall-test-" + strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-")
	// The image runs as a non-root user, so it must be able to write the
	// mounted directory.
	if err := os.Chmod(data, 0o777); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("docker", "run", "--rm", "--name", name, "-p", "127.0.0.1:0:8471",
		"-v", data+":/data", "-e", "TZ=Australia/Adelaide", image, "serve")
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	in := &instance{t: t, data: data, cmd: cmd, out: bufio.NewScanner(stdout)}
	t.Cleanup(in.stop)
	first := in.next()
	if !strings.HasPrefix(first, "level=INFO msg=\"recall serving\" ") {
		t.Fatalf("unexpected first line: %s", first)
	}
	// The container logs its own port; ask Docker for the published one.
	out, err := exec.Command("docker", "port", name, "8471/tcp").Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}
	in.url = "http://" + strings.TrimSpace(strings.Split(string(out), "\n")[0])
	return in
}

// stop asks the container to shut down and waits for it; docker run
// forwards the interrupt.
func (in *instance) stop() {
	if in.cmd.ProcessState != nil {
		return
	}
	in.cmd.Process.Signal(os.Interrupt)
	in.cmd.Wait()
}

// next returns the next log line with its timestamp removed, so the rest
// can be compared exactly.
func (in *instance) next() string {
	if !in.out.Scan() {
		in.t.Fatalf("binary stopped logging; seen:\n%s", strings.Join(in.seen, "\n"))
	}
	line := in.out.Text()
	in.seen = append(in.seen, line)
	i := strings.Index(line, " level=")
	if i < 0 {
		in.t.Fatalf("not a log line: %s", line)
	}
	return line[i+1:]
}

func (in *instance) post(name string) int {
	in.t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "internal", "webhook", "testdata", name+".json"))
	if err != nil {
		in.t.Fatal(err)
	}
	resp, err := http.Post(in.url+"/webhook", "application/json", bytes.NewReader(raw))
	if err != nil {
		in.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func (in *instance) lines(file string) int {
	raw, err := os.ReadFile(filepath.Join(in.data, file))
	if err != nil {
		in.t.Fatal(err)
	}
	return bytes.Count(raw, []byte("\n"))
}

func TestReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	in := start(t, t.TempDir())

	// Every captured body, in the order it arrived, with the line the binary
	// must log for it.
	steps := []struct {
		body string
		log  string
	}{
		{"radarr-test", `level=INFO msg=ignored instance=Radarr event=Test`},
		{"radarr-movie-added", `level=INFO msg=ignored instance=Radarr event=MovieAdded`},
		{"radarr-grab", `level=INFO msg=grab instance=Radarr media="100% Wolf (2020)" release="100 Percent Wolf 2020 1080p BluRay DD5.1 x264-PTer" score=881400 download=0000000000000000000000000000000000000001`},
		{"radarr-import", `level=INFO msg=decision result=drift instance=Radarr media="100% Wolf (2020)" release="100 Percent Wolf 2020 1080p BluRay DD5.1 x264-PTer" file="100 Percent Wolf.2020.1080p.BluRay.DD5.1.x264- PTer" grabScore=881400 importScore=-299599 delta=-1180999 lost="[1080p Quality Tier 5]" gained="[Release Group (Missing)]" path="/media/test-library/100% Wolf (2020) {tmdb-520946}/100% Wolf (2020) {tmdb-520946} [Bluray-1080p][AC3 5.1][x264].mkv" download=0000000000000000000000000000000000000001`},
		{"sonarr-test", `level=INFO msg=ignored instance=Sonarr event=Test`},
		{"sonarr-series-add", `level=INFO msg=ignored instance=Sonarr event=SeriesAdd`},
		{"sonarr-health", `level=INFO msg=ignored instance=Sonarr event=Health`},
		{"sonarr-grab-pack-no-group", `level=INFO msg=grab instance=Sonarr media="Sherlock (2010) S01E01,S01E02,S01E03" release="Sherlock S01 Complete 720p BRRip x264 AAC - M@X" score=540210 download=0000000000000000000000000000000000000002`},
		{"sonarr-grab-episode", `level=INFO msg=grab instance=Sonarr media="Sherlock (2010) S02E01" release="Sherlock S02E01 A Scandal in Belgravia 1080p NF WEB-DL DD 5 1 H 264-playWEB" score=861480 download=0000000000000000000000000000000000000003`},
		{"sonarr-grab-pack", `level=INFO msg=grab instance=Sonarr media="Sherlock (2010) S01E01,S01E02,S01E03" release="Sherlock (2010) S01 (1080p AMZN WEB-DL H265 SDR DDP 5 1 English - HONE)" score=923690 download=0000000000000000000000000000000000000004`},
		{"sonarr-import-pack-e01", `level=INFO msg=decision result=clean instance=Sonarr media="Sherlock (2010) S01E01" release="Sherlock (2010) S01 (1080p AMZN WEB-DL H265 SDR DDP 5 1 English - HONE)" file="Sherlock (2010) - S01E01 - A Study in Pink [AMZN][WEBDL-1080p][EAC3 5.1][h265]-HONE.mkv" grabScore=923690 importScore=923690 delta=0 lost=[] gained=[] path="/media/test-library/Sherlock (2010) {tvdb-176941}/Season 01/Sherlock (2010) - S01E01 - A Study in Pink [AMZN][WEBDL-1080p][EAC3 5.1][h265]-HONE.mkv" download=0000000000000000000000000000000000000004`},
		{"sonarr-import-pack-e02", `level=INFO msg=decision result=clean instance=Sonarr media="Sherlock (2010) S01E02" release="Sherlock (2010) S01 (1080p AMZN WEB-DL H265 SDR DDP 5 1 English - HONE)" file="Sherlock (2010) - S01E02 - The Blind Banker [AMZN][WEBDL-1080p][EAC3 5.1][h265]-HONE.mkv" grabScore=923690 importScore=923690 delta=0 lost=[] gained=[] path="/media/test-library/Sherlock (2010) {tvdb-176941}/Season 01/Sherlock (2010) - S01E02 - The Blind Banker [AMZN][WEBDL-1080p][EAC3 5.1][h265]-HONE.mkv" download=0000000000000000000000000000000000000004`},
		{"sonarr-import-pack-e03", `level=INFO msg=decision result=clean instance=Sonarr media="Sherlock (2010) S01E03" release="Sherlock (2010) S01 (1080p AMZN WEB-DL H265 SDR DDP 5 1 English - HONE)" file="Sherlock (2010) - S01E03 - The Great Game [AMZN][WEBDL-1080p][EAC3 5.1][h265]-HONE.mkv" grabScore=923690 importScore=923690 delta=0 lost=[] gained=[] path="/media/test-library/Sherlock (2010) {tvdb-176941}/Season 01/Sherlock (2010) - S01E03 - The Great Game [AMZN][WEBDL-1080p][EAC3 5.1][h265]-HONE.mkv" download=0000000000000000000000000000000000000004`},
		{"sonarr-import-pack-summary", `level=INFO msg=ignored instance=Sonarr event=Download`},
		{"sonarr-import-episode", `level=INFO msg=decision result=clean instance=Sonarr media="Sherlock (2010) S02E01" release="Sherlock S02E01 A Scandal in Belgravia 1080p NF WEB-DL DD 5 1 H 264-playWEB" file="Sherlock S02E01 A Scandal in Belgravia 1080p NF WEB-DL DD 5 1 H 264-playWEB" grabScore=861480 importScore=861480 delta=0 lost=[] gained=[] path="/media/test-library/Sherlock (2010) {tvdb-176941}/Season 02/Sherlock (2010) - S02E01 - A Scandal in Belgravia [NF][WEBDL-1080p][EAC3 5.1][x264]-playWEB.mkv" download=0000000000000000000000000000000000000003`},
		{"sonarr-import-episode-summary", `level=INFO msg=ignored instance=Sonarr event=Download`},
	}
	for _, s := range steps {
		if code := in.post(s.body); code != http.StatusAccepted {
			t.Fatalf("%s: HTTP %d", s.body, code)
		}
		if got := in.next(); got != s.log {
			t.Errorf("%s:\n got %s\nwant %s", s.body, got, s.log)
		}
	}

	if g, i, d := in.lines("grabs.jsonl"), in.lines("imports.jsonl"), in.lines("decisions.jsonl"); g != 4 || i != 5 || d != 5 {
		t.Errorf("files: %d grabs, %d imports, %d decisions; want 4, 5, 5", g, i, d)
	}
}

func TestRestartKeepsGrabs(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	data := t.TempDir()
	in := start(t, data)
	in.post("sonarr-grab-pack")
	in.next()
	in.stop()

	in = start(t, data)
	if !strings.HasSuffix(in.seen[0], " grabs=1") {
		t.Errorf("restart did not reload the grab: %s", in.seen[0])
	}
	in.post("sonarr-import-pack-e02")
	if got := in.next(); !strings.Contains(got, "result=clean") || !strings.Contains(got, "grabScore=923690") {
		t.Errorf("import after restart: %s", got)
	}
}

func TestBadRequestsAreNotLogged(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	in := start(t, t.TempDir())
	resp, err := http.Post(in.url+"/webhook", "application/json", strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("got %d", resp.StatusCode)
	}
	// A good one after it must be the very next line, so the bad one wrote nothing.
	in.post("radarr-test")
	if got := in.next(); got != `level=INFO msg=ignored instance=Radarr event=Test` {
		t.Errorf("got %s", got)
	}
}
