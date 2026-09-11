//go:build integration

package test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santiagosayshey/recall/internal/arr"
	"github.com/santiagosayshey/recall/internal/config"
	"github.com/santiagosayshey/recall/internal/register"
)

// The registration test brings up compose.yml, a real Radarr and Sonarr
// beside the image under test, and checks that Recall made its connection
// on each, that the apps can reach it through that connection, and that
// nothing without the secret gets in.

const (
	radarrKey = "0123456789abcdef0123456789abcdef"
	sonarrKey = "fedcba9876543210fedcba9876543210"
	secret    = "recall-test-secret"
)

type stack struct {
	t       *testing.T
	project string
}

func compose(t *testing.T) *stack {
	t.Helper()
	s := &stack{t: t, project: fmt.Sprintf("recall-test-%d", time.Now().UnixNano()%1e6)}
	t.Cleanup(func() {
		out, err := s.run("down", "-v", "--timeout", "5")
		if err != nil {
			t.Logf("compose down: %v\n%s", err, out)
		}
	})
	// The registry that serves the Radarr and Sonarr images rate-limits
	// pulls now and then, so pulling gets a few tries before it counts.
	var out string
	var err error
	for attempt := 1; attempt <= 5; attempt++ {
		if out, err = s.run("pull", "--quiet", "radarr", "sonarr"); err == nil {
			break
		}
		t.Logf("compose pull attempt %d: %v\n%s", attempt, err, out)
		time.Sleep(time.Duration(attempt*attempt) * 5 * time.Second)
	}
	if err != nil {
		t.Fatalf("compose pull: %v\n%s", err, out)
	}
	if out, err := s.run("up", "-d", "--quiet-pull"); err != nil {
		t.Fatalf("compose up: %v\n%s", err, out)
	}
	return s
}

func (s *stack) run(args ...string) (string, error) {
	cmd := exec.Command("docker", append([]string{"compose", "-f", "compose.yml", "-p", s.project}, args...)...)
	cmd.Env = append(os.Environ(), "RECALL_IMAGE="+image)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// url returns the published address of a service port.
func (s *stack) url(service string, port int) string {
	out, err := s.run("port", service, fmt.Sprint(port))
	if err != nil {
		s.t.Fatalf("compose port %s: %v\n%s", service, err, out)
	}
	return "http://" + strings.TrimSpace(strings.Split(out, "\n")[0])
}

// logs returns everything the Recall container has logged so far, with
// the compose prefix and the timestamp stripped.
func (s *stack) logs() []string {
	out, err := s.run("logs", "--no-log-prefix", "recall")
	if err != nil {
		s.t.Fatalf("compose logs: %v\n%s", err, out)
	}
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if i := strings.Index(sc.Text(), " level="); i >= 0 {
			lines = append(lines, sc.Text()[i+1:])
		}
	}
	return lines
}

// waitFor polls until the Recall log contains every wanted line.
func (s *stack) waitFor(want ...string) []string {
	deadline := time.Now().Add(3 * time.Minute)
	for {
		lines := s.logs()
		missing := 0
		for _, w := range want {
			found := false
			for _, l := range lines {
				if l == w {
					found = true
					break
				}
			}
			if !found {
				missing++
			}
		}
		if missing == 0 {
			return lines
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("waited 3 minutes for %q; log:\n%s", want, strings.Join(lines, "\n"))
		}
		time.Sleep(2 * time.Second)
	}
}

func waitForApp(t *testing.T, c *arr.Client) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := c.InstanceName(ctx)
		cancel()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("app not up after 3 minutes: %v", err)
		}
		time.Sleep(2 * time.Second)
	}
}

func TestRegister(t *testing.T) {
	s := compose(t)
	ctx := context.Background()
	apps := []struct {
		app, name, key string
		port           int
	}{
		{"radarr", "Radarr Test", radarrKey, 7878},
		{"sonarr", "Sonarr Test", sonarrKey, 8989},
	}
	clients := map[string]*arr.Client{}
	for _, a := range apps {
		c, err := arr.New(s.url(a.app, a.port), a.key)
		if err != nil {
			t.Fatal(err)
		}
		waitForApp(t, c)
		clients[a.app] = c
	}

	// Recall registers on start, retrying until both apps are up.
	s.waitFor(
		`level=INFO msg=registered instance="Radarr Test" connection=created`,
		`level=INFO msg=registered instance="Sonarr Test" connection=created`,
	)

	// Ask each app for its connections and check ours field by field.
	recallURL := "http://recall:8471/webhook"
	for _, a := range apps {
		c := clients[a.app]
		name, err := c.InstanceName(ctx)
		if err != nil || name != a.name {
			t.Errorf("%s calls itself %q, %v", a.app, name, err)
		}
		list, err := c.Notifications(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var ours []arr.Notification
		for _, n := range list {
			if n.Name == register.Name {
				ours = append(ours, n)
			}
		}
		if len(ours) != 1 {
			t.Fatalf("%s has %d Recall connections: %+v", a.app, len(ours), list)
		}
		want := register.Desired(a.app, recallURL, secret)
		if !register.Matches(a.app, ours[0], want) {
			t.Errorf("%s connection is not what Recall wants:\n%+v", a.app, ours[0])
		}
		if h := fmt.Sprint(ours[0].Field("headers")); !strings.Contains(h, config.SecretHeader) || !strings.Contains(h, secret) {
			t.Errorf("%s connection lacks the secret header: %s", a.app, h)
		}

		// The app's own connection test posts to Recall for real; it only
		// passes if Recall answered, which needs the secret to be right.
		if err := c.Test(ctx, ours[0]); err != nil {
			t.Errorf("%s connection test: %v", a.app, err)
		}
	}
	s.waitFor(
		`level=INFO msg=ignored instance="Radarr Test" event=Test`,
		`level=INFO msg=ignored instance="Sonarr Test" event=Test`,
	)

	// Straight to Recall: nothing without the secret, everything with it.
	raw, err := os.ReadFile(filepath.Join("..", "internal", "webhook", "testdata", "radarr-grab.json"))
	if err != nil {
		t.Fatal(err)
	}
	hook := s.url("recall", 8471) + "/webhook"
	post := func(header string) int {
		req, _ := http.NewRequest(http.MethodPost, hook, bytes.NewReader(raw))
		if header != "" {
			req.Header.Set(config.SecretHeader, header)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := post(""); got != http.StatusUnauthorized {
		t.Errorf("no secret: %d", got)
	}
	if got := post("wrong"); got != http.StatusUnauthorized {
		t.Errorf("wrong secret: %d", got)
	}
	if got := post(secret); got != http.StatusAccepted {
		t.Errorf("right secret: %d", got)
	}
	s.waitFor(`level=INFO msg=grab instance=Radarr media="100% Wolf (2020)" release="100 Percent Wolf 2020 1080p BluRay DD5.1 x264-PTer" score=881400 download=0000000000000000000000000000000000000001`)

	// A second registration changes nothing.
	out, err := s.run("run", "--rm", "--no-deps", "recall", "register")
	if err != nil {
		t.Fatalf("register again: %v\n%s", err, out)
	}
	for _, name := range []string{"Radarr Test", "Sonarr Test"} {
		if !strings.Contains(out, fmt.Sprintf(`msg=registered instance=%q connection=unchanged`, name)) {
			t.Errorf("second register for %s was not unchanged:\n%s", name, out)
		}
	}
	for _, a := range apps {
		list, _ := clients[a.app].Notifications(ctx)
		if len(list) != 1 {
			t.Errorf("%s now has %d connections", a.app, len(list))
		}
	}
}
