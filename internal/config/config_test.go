package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const good = `
url: http://recall:8471/webhook
secret: ${RECALL_TEST_SECRET}
instances:
  - name: Radarr Test
    app: radarr
    url: http://radarr:7878
    apiKey: ${RADARR_TEST_KEY}
  - name: Sonarr Test
    app: sonarr
    url: http://sonarr:8989
    apiKey: literal-key
`

func TestParse(t *testing.T) {
	t.Setenv("RECALL_TEST_SECRET", "s3cret")
	t.Setenv("RADARR_TEST_KEY", "abc")
	c, err := Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if c.URL != "http://recall:8471/webhook" || c.Secret != "s3cret" || len(c.Instances) != 2 {
		t.Fatalf("%+v", c)
	}
	if c.Instances[0].APIKey != "abc" || c.Instances[1].APIKey != "literal-key" || c.Instances[0].Name != "Radarr Test" {
		t.Fatalf("%+v", c.Instances)
	}
}

func TestLoad(t *testing.T) {
	t.Setenv("RECALL_TEST_SECRET", "s")
	t.Setenv("RADARR_TEST_KEY", "k")
	path := filepath.Join(t.TempDir(), "config.yml")
	os.WriteFile(path, []byte(good), 0o644)
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yml")); err == nil {
		t.Error("loaded a missing file")
	}
}

func TestParseErrors(t *testing.T) {
	t.Setenv("RADARR_TEST_KEY", "")
	tests := []struct {
		name string
		yaml string
		want []string // substrings of the error, all reported at once
	}{
		{"empty", ``, []string{"url: required", "secret: required", "instances: at least one"}},
		{"unset key", good, []string{"instances[0]: apiKey: required", "secret: required"}},
		{"bad urls", "url: recall\nsecret: s\ninstances:\n  - {name: A, app: radarr, url: ftp://x, apiKey: k}\n",
			[]string{`url: "recall" is not an http or https URL`, `instances[0]: url: "ftp://x" is not an http`}},
		{"bad app and duplicate", "url: http://r/\nsecret: s\ninstances:\n  - {name: A, app: lidarr, url: http://x, apiKey: k}\n  - {name: A, app: sonarr, url: http://y, apiKey: k}\n",
			[]string{`instances[0]: app: "lidarr" is not radarr or sonarr`, `instances[1]: name: "A" appears twice`}},
		{"unknown key", "url: http://r/\nsecret: s\ntopic: x\ninstances: []\n", []string{"field topic not found"}},
		{"not yaml", "{", []string{"config:"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil {
				t.Fatal("no error")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error lacks %q:\n%v", w, err)
				}
			}
		})
	}
}
