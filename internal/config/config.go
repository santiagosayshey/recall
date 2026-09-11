// Package config reads config.yml: the URL the *arr apps post to, the
// secret they must send, and the instances Recall registers itself on.
// Values may reference environment variables as ${NAME}, so API keys can
// live in the environment while the file is tracked.
package config

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// SecretHeader is the header the apps send and the server checks.
const SecretHeader = "X-Recall-Secret"

type Config struct {
	URL       string     `yaml:"url"`    // where the apps post, for example http://recall:8471/webhook
	Secret    string     `yaml:"secret"` // sent by the apps as SecretHeader and checked on every event
	Instances []Instance `yaml:"instances"`
}

type Instance struct {
	Name   string `yaml:"name"`   // must equal the app's own instance name; events are keyed on it
	App    string `yaml:"app"`    // radarr or sonarr
	URL    string `yaml:"url"`    // the app's address as Recall reaches it
	APIKey string `yaml:"apiKey"` // usually ${SOME_VAR}
}

// Load reads and checks the file. Every problem is reported at once.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return Parse(raw)
}

// Parse reads config.yml from memory, expanding ${NAME} from the
// environment first.
func Parse(raw []byte) (Config, error) {
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(os.ExpandEnv(string(raw))))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	var errs []error
	fail := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	if err := checkURL(c.URL); err != nil {
		fail("url: %v", err)
	}
	if c.Secret == "" {
		fail("secret: required")
	}
	if len(c.Instances) == 0 {
		fail("instances: at least one")
	}
	seen := map[string]bool{}
	for i, in := range c.Instances {
		at := fmt.Sprintf("instances[%d]", i)
		if in.Name == "" {
			fail("%s: name: required", at)
		} else if seen[in.Name] {
			fail("%s: name: %q appears twice", at, in.Name)
		}
		seen[in.Name] = true
		if in.App != "radarr" && in.App != "sonarr" {
			fail("%s: app: %q is not radarr or sonarr", at, in.App)
		}
		if err := checkURL(in.URL); err != nil {
			fail("%s: url: %v", at, err)
		}
		if in.APIKey == "" {
			fail("%s: apiKey: required (is the environment variable set?)", at)
		}
	}
	if len(errs) > 0 {
		return Config{}, fmt.Errorf("config: %w", errors.Join(errs...))
	}
	return c, nil
}

func checkURL(s string) error {
	if s == "" {
		return errors.New("required")
	}
	u, err := url.Parse(s)
	if err != nil {
		return err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%q is not an http or https URL", s)
	}
	return nil
}
