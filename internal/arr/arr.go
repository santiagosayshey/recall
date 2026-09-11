// Package arr is the little of Radarr's and Sonarr's API that Recall needs:
// who the instance says it is, and its list of notification connections.
// Both apps serve the same shape at /api/v3.
package arr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	base string
	key  string
	http *http.Client
}

// New returns a client for one instance. The URL is the app's root, for
// example http://radarr:7878, with or without a URL base.
func New(baseURL, apiKey string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("arr: %q is not an http or https URL", baseURL)
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), key: apiKey, http: &http.Client{Timeout: 15 * time.Second}}, nil
}

// Notification is a connection as the API shows it. Only what Recall reads
// or writes is typed; the rest rides along in Extra so an update sends back
// everything the app gave us.
type Notification struct {
	ID             int            `json:"id,omitempty"`
	Name           string         `json:"name"`
	Implementation string         `json:"implementation"`
	ConfigContract string         `json:"configContract"`
	Fields         []Field        `json:"fields"`
	Tags           []int          `json:"tags"`
	Extra          map[string]any `json:"-"` // every other key, including the on* flags
}

type Field struct {
	Name  string `json:"name"`
	Value any    `json:"value,omitempty"`
}

// Field returns the value of a named field, or nil.
func (n Notification) Field(name string) any {
	for _, f := range n.Fields {
		if f.Name == name {
			return f.Value
		}
	}
	return nil
}

func (n Notification) MarshalJSON() ([]byte, error) {
	type plain Notification
	m := map[string]any{}
	for k, v := range n.Extra {
		m[k] = v
	}
	b, err := json.Marshal(plain(n))
	if err != nil {
		return nil, err
	}
	var typed map[string]any
	if err := json.Unmarshal(b, &typed); err != nil {
		return nil, err
	}
	for k, v := range typed {
		m[k] = v
	}
	return json.Marshal(m)
}

func (n *Notification) UnmarshalJSON(b []byte) error {
	type plain Notification
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	var all map[string]any
	if err := json.Unmarshal(b, &all); err != nil {
		return err
	}
	for _, k := range []string{"id", "name", "implementation", "configContract", "fields", "tags"} {
		delete(all, k)
	}
	*n = Notification(p)
	n.Extra = all
	return nil
}

// InstanceName asks the app what it calls itself.
func (c *Client) InstanceName(ctx context.Context) (string, error) {
	var status struct {
		InstanceName string `json:"instanceName"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v3/system/status", nil, &status); err != nil {
		return "", err
	}
	return status.InstanceName, nil
}

// Notifications lists every connection.
func (c *Client) Notifications(ctx context.Context) ([]Notification, error) {
	var list []Notification
	err := c.do(ctx, http.MethodGet, "/api/v3/notification", nil, &list)
	return list, err
}

// Create adds a connection and returns it as the app stored it.
func (c *Client) Create(ctx context.Context, n Notification) (Notification, error) {
	var out Notification
	err := c.do(ctx, http.MethodPost, "/api/v3/notification", n, &out)
	return out, err
}

// Update replaces the connection with n's id.
func (c *Client) Update(ctx context.Context, n Notification) (Notification, error) {
	var out Notification
	err := c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v3/notification/%d", n.ID), n, &out)
	return out, err
}

// Test asks the app to send its test event through the connection.
func (c *Client) Test(ctx context.Context, n Notification) error {
	return c.do(ctx, http.MethodPost, "/api/v3/notification/test", n, nil)
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.key)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, firstLine(raw))
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	return nil
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
