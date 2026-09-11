package arr

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/santiagosayshey/recall/internal/arr/arrtest"
)

func TestNew(t *testing.T) {
	for _, bad := range []string{"", "radarr:7878", "ftp://x", "http://"} {
		if _, err := New(bad, "k"); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	c, err := New("http://radarr:7878/", "k")
	if err != nil || c.base != "http://radarr:7878" {
		t.Fatalf("%v %q", err, c.base)
	}
}

func TestClient(t *testing.T) {
	fake := arrtest.New("Radarr Test", "key")
	defer fake.Close()
	ctx := context.Background()
	c, _ := New(fake.URL, "key")

	name, err := c.InstanceName(ctx)
	if err != nil || name != "Radarr Test" {
		t.Fatalf("%q %v", name, err)
	}
	list, err := c.Notifications(ctx)
	if err != nil || len(list) != 0 {
		t.Fatalf("%v %v", list, err)
	}
	n := Notification{Name: "Recall", Implementation: "Webhook", ConfigContract: "WebhookSettings",
		Fields: []Field{{Name: "url", Value: "http://recall/webhook"}, {Name: "method", Value: 1}, {Name: "password", Value: "p"}},
		Tags:   []int{}, Extra: map[string]any{"onGrab": true, "onRename": false}}
	created, err := c.Create(ctx, n)
	if err != nil || created.ID != 1 || created.Extra["onGrab"] != true || created.Field("url") != "http://recall/webhook" {
		t.Fatalf("%+v %v", created, err)
	}
	if created.Field("password") != nil {
		t.Error("the app should have hidden the password")
	}
	if created.Field("method") != float64(1) {
		t.Errorf("method came back as %T %v", created.Field("method"), created.Field("method"))
	}
	created.Fields[0].Value = "http://other/webhook"
	updated, err := c.Update(ctx, created)
	if err != nil || updated.ID != 1 || updated.Field("url") != "http://other/webhook" {
		t.Fatalf("%+v %v", updated, err)
	}
	if err := c.Test(ctx, updated); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Update(ctx, Notification{ID: 99}); err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("missing id: %v", err)
	}
	bad, _ := New(fake.URL, "wrong")
	if _, err := bad.InstanceName(ctx); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("wrong key: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := c.Notifications(cancelled); err == nil {
		t.Error("cancelled context accepted")
	}
}

// Extra keys survive a round trip so an update sends back everything the
// app gave, and the typed keys are not duplicated into Extra.
func TestNotificationJSON(t *testing.T) {
	src := `{"id":3,"name":"Recall","implementation":"Webhook","configContract":"WebhookSettings","onGrab":true,"supportsOnGrab":true,"fields":[{"name":"url","value":"u"}],"tags":[]}`
	var n Notification
	if err := json.Unmarshal([]byte(src), &n); err != nil {
		t.Fatal(err)
	}
	if n.ID != 3 || n.Extra["onGrab"] != true || n.Extra["supportsOnGrab"] != true || n.Extra["name"] != nil || n.Extra["fields"] != nil {
		t.Fatalf("%+v", n)
	}
	out, _ := json.Marshal(n)
	var m map[string]any
	json.Unmarshal(out, &m)
	if m["id"] != float64(3) || m["onGrab"] != true || m["supportsOnGrab"] != true || m["name"] != "Recall" {
		t.Fatalf("%s", out)
	}
}
