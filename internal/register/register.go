// Package register makes each instance's Recall connection what Recall
// wants it to be, and proves it by reading it back.
package register

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/santiagosayshey/recall/internal/arr"
	"github.com/santiagosayshey/recall/internal/config"
)

// Name is the connection's name in the app's Connect page.
const Name = "Recall"

// flags are every on* switch each app has. All are sent, so nothing is
// left to the app's default, and only grabs, imports and upgrades are on.
var flags = map[string][]string{
	"radarr": {"onGrab", "onDownload", "onUpgrade", "onRename", "onMovieAdded", "onMovieDelete", "onMovieFileDelete", "onMovieFileDeleteForUpgrade", "onHealthIssue", "onHealthRestored", "onApplicationUpdate", "onManualInteractionRequired"},
	"sonarr": {"onGrab", "onDownload", "onUpgrade", "onImportComplete", "onRename", "onSeriesAdd", "onSeriesDelete", "onEpisodeFileDelete", "onEpisodeFileDeleteForUpgrade", "onHealthIssue", "onHealthRestored", "onApplicationUpdate", "onManualInteractionRequired"},
}

var wanted = map[string]bool{"onGrab": true, "onDownload": true, "onUpgrade": true}

// Desired is the connection Recall wants on an app.
func Desired(app, url, secret string) arr.Notification {
	extra := map[string]any{"includeHealthWarnings": false}
	for _, f := range flags[app] {
		extra[f] = wanted[f]
	}
	return arr.Notification{
		Name:           Name,
		Implementation: "Webhook",
		ConfigContract: "WebhookSettings",
		Fields: []arr.Field{
			{Name: "url", Value: url},
			{Name: "method", Value: 1}, // POST
			{Name: "username", Value: ""},
			{Name: "password", Value: ""},
			{Name: "headers", Value: []map[string]string{{"key": config.SecretHeader, "value": secret}}},
		},
		Tags:  []int{},
		Extra: extra,
	}
}

// Matches reports whether a connection as the app shows it is the desired
// one: same name, kind, flags, URL, method and headers. Username and
// password are not compared because the app hides them.
func Matches(app string, have, want arr.Notification) bool {
	if have.Name != want.Name || have.Implementation != want.Implementation || have.ConfigContract != want.ConfigContract {
		return false
	}
	for _, f := range flags[app] {
		if v, _ := have.Extra[f].(bool); v != wanted[f] {
			return false
		}
	}
	if v, _ := have.Extra["includeHealthWarnings"].(bool); v {
		return false
	}
	if len(have.Tags) != 0 {
		return false
	}
	for _, name := range []string{"url", "method", "headers"} {
		if !same(have.Field(name), want.Field(name)) {
			return false
		}
	}
	return true
}

// same compares two field values through JSON, since the app returns
// numbers as float64 and lists as []any.
func same(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	var va, vb any
	json.Unmarshal(ja, &va)
	json.Unmarshal(jb, &vb)
	return reflect.DeepEqual(va, vb)
}

// Action is what Instance had to do.
type Action string

const (
	Created   Action = "created"
	Updated   Action = "updated"
	Unchanged Action = "unchanged"
)

// Instance registers Recall on one app. It first checks the app calls
// itself by the configured name, since events are keyed on that, then
// creates or updates the connection only if needed, then reads it back and
// fails unless it is exactly what was wanted.
func Instance(ctx context.Context, c *arr.Client, in config.Instance, url, secret string) (Action, error) {
	name, err := c.InstanceName(ctx)
	if err != nil {
		return "", err
	}
	if name != in.Name {
		return "", fmt.Errorf("the app calls itself %q, the config says %q", name, in.Name)
	}
	want := Desired(in.App, url, secret)
	have, found, err := find(ctx, c)
	if err != nil {
		return "", err
	}
	action := Unchanged
	switch {
	case !found:
		if _, err := c.Create(ctx, want); err != nil {
			return "", fmt.Errorf("create: %w", err)
		}
		action = Created
	case !Matches(in.App, have, want):
		want.ID = have.ID
		if _, err := c.Update(ctx, want); err != nil {
			return "", fmt.Errorf("update: %w", err)
		}
		action = Updated
	}
	back, found, err := find(ctx, c)
	if err != nil {
		return "", err
	}
	if !found {
		return "", errors.New("read back: the connection is not there")
	}
	if !Matches(in.App, back, want) {
		b, _ := json.Marshal(back)
		return "", fmt.Errorf("read back: the connection is not what was written: %s", b)
	}
	return action, nil
}

func find(ctx context.Context, c *arr.Client) (arr.Notification, bool, error) {
	list, err := c.Notifications(ctx)
	if err != nil {
		return arr.Notification{}, false, err
	}
	for _, n := range list {
		if n.Name == Name {
			return n, true, nil
		}
	}
	return arr.Notification{}, false, nil
}

// Result is one instance's outcome.
type Result struct {
	Instance string
	Action   Action
	Err      error
}

// All registers every instance in the config and returns one result each.
// It does not stop at the first failure.
func All(ctx context.Context, cfg config.Config) []Result {
	results := make([]Result, 0, len(cfg.Instances))
	for _, in := range cfg.Instances {
		r := Result{Instance: in.Name}
		c, err := arr.New(in.URL, in.APIKey)
		if err != nil {
			r.Err = err
		} else {
			r.Action, r.Err = Instance(ctx, c, in, cfg.URL, cfg.Secret)
		}
		results = append(results, r)
	}
	return results
}
