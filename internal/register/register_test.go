package register

import (
	"context"
	"strings"
	"testing"

	"github.com/santiagosayshey/recall/internal/arr"
	"github.com/santiagosayshey/recall/internal/arr/arrtest"
	"github.com/santiagosayshey/recall/internal/config"
)

const (
	hook   = "http://recall:8471/webhook"
	secret = "s3cret"
)

func setup(t *testing.T, app, name string) (*arrtest.Server, *arr.Client, config.Instance) {
	t.Helper()
	fake := arrtest.New(name, "key")
	t.Cleanup(fake.Close)
	c, _ := arr.New(fake.URL, "key")
	return fake, c, config.Instance{Name: name, App: app, URL: fake.URL, APIKey: "key"}
}

func calls(fake *arrtest.Server, method string) int {
	n := 0
	for _, c := range fake.Calls {
		if strings.HasPrefix(c, method+" ") {
			n++
		}
	}
	return n
}

func TestInstanceIsIdempotent(t *testing.T) {
	ctx := context.Background()
	for _, app := range []string{"radarr", "sonarr"} {
		t.Run(app, func(t *testing.T) {
			fake, c, in := setup(t, app, "Test")
			got, err := Instance(ctx, c, in, hook, secret)
			if err != nil || got != Created {
				t.Fatalf("first: %s %v", got, err)
			}
			list := fake.Notifications()
			if len(list) != 1 || list[0]["name"] != Name || list[0]["onGrab"] != true || list[0]["onDownload"] != true || list[0]["onUpgrade"] != true || list[0]["onRename"] != false {
				t.Fatalf("stored: %v", list)
			}
			for _, f := range flags[app] {
				if _, ok := list[0][f]; !ok {
					t.Errorf("flag %s not sent", f)
				}
			}

			writes := calls(fake, "POST") + calls(fake, "PUT")
			got, err = Instance(ctx, c, in, hook, secret)
			if err != nil || got != Unchanged {
				t.Fatalf("second: %s %v", got, err)
			}
			if calls(fake, "POST")+calls(fake, "PUT") != writes {
				t.Error("second run wrote something")
			}

			got, err = Instance(ctx, c, in, hook, "rotated")
			if err != nil || got != Updated {
				t.Fatalf("after secret change: %s %v", got, err)
			}
			if len(fake.Notifications()) != 1 {
				t.Error("update made a second connection")
			}
			got, err = Instance(ctx, c, in, "http://elsewhere/webhook", "rotated")
			if err != nil || got != Updated {
				t.Fatalf("after url change: %s %v", got, err)
			}
		})
	}
}

func TestInstanceRefusesWrongName(t *testing.T) {
	_, c, in := setup(t, "radarr", "Radarr")
	in.Name = "Radarr Test"
	_, err := Instance(context.Background(), c, in, hook, secret)
	if err == nil || !strings.Contains(err.Error(), `calls itself "Radarr", the config says "Radarr Test"`) {
		t.Fatalf("%v", err)
	}
}

func TestInstanceFailsWhenReadBackDiffers(t *testing.T) {
	fake, c, in := setup(t, "radarr", "Test")
	fake.IgnoreWrites = true
	_, err := Instance(context.Background(), c, in, hook, secret)
	if err == nil || !strings.Contains(err.Error(), "read back") {
		t.Fatalf("%v", err)
	}
}

func TestInstanceErrors(t *testing.T) {
	fake, _, in := setup(t, "radarr", "Test")
	wrong, _ := arr.New(fake.URL, "wrong")
	if _, err := Instance(context.Background(), wrong, in, hook, secret); err == nil {
		t.Error("wrong key accepted")
	}
	fake.Close()
	c, _ := arr.New(fake.URL, "key")
	if _, err := Instance(context.Background(), c, in, hook, secret); err == nil {
		t.Error("dead app accepted")
	}
}

func TestMatchesIgnoresWhatTheAppAdds(t *testing.T) {
	want := Desired("radarr", hook, secret)
	have := Desired("radarr", hook, secret)
	have.ID = 7
	have.Extra["supportsOnGrab"] = true
	have.Extra["infoLink"] = "https://wiki"
	have.Fields = append(have.Fields[:3:3], arr.Field{Name: "headers", Value: []any{map[string]any{"key": config.SecretHeader, "value": secret}}})
	have.Fields[1].Value = float64(1)
	if !Matches("radarr", have, want) {
		t.Error("the app's additions should not count as a difference")
	}
	for _, change := range []func(*arr.Notification){
		func(n *arr.Notification) { n.Name = "Other" },
		func(n *arr.Notification) { n.Implementation = "Slack" },
		func(n *arr.Notification) { n.Extra["onRename"] = true },
		func(n *arr.Notification) { n.Extra["onGrab"] = false },
		func(n *arr.Notification) { n.Extra["includeHealthWarnings"] = true },
		func(n *arr.Notification) { n.Tags = []int{1} },
		func(n *arr.Notification) { n.Fields[0].Value = "http://x" },
		func(n *arr.Notification) { n.Fields[1].Value = float64(2) },
		func(n *arr.Notification) { n.Fields[4].Value = []any{} },
	} {
		h := Desired("radarr", hook, secret)
		change(&h)
		if Matches("radarr", h, want) {
			t.Errorf("change not noticed: %+v", h)
		}
	}
}

func TestAll(t *testing.T) {
	r, _, rin := setup(t, "radarr", "Radarr Test")
	_, _, sin := setup(t, "sonarr", "Sonarr Test")
	sin.Name = "Wrong"
	cfg := config.Config{URL: hook, Secret: secret, Instances: []config.Instance{rin, sin, {Name: "Bad", App: "radarr", URL: "nope", APIKey: "k"}}}
	results := All(context.Background(), cfg)
	if len(results) != 3 || results[0].Err != nil || results[0].Action != Created || results[1].Err == nil || results[2].Err == nil {
		t.Fatalf("%+v", results)
	}
	if len(r.Notifications()) != 1 {
		t.Error("radarr not registered")
	}
}
