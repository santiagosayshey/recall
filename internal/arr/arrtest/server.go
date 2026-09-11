// Package arrtest is a fake Radarr or Sonarr: an instance name, an API key,
// and a notification list that behaves like the real one, for tests.
package arrtest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
)

type Server struct {
	*httptest.Server
	InstanceName string
	APIKey       string
	// IgnoreWrites makes creates and updates answer 2xx without storing,
	// so a read-back mismatch can be provoked.
	IgnoreWrites bool

	mu     sync.Mutex
	nextID int
	list   []map[string]any
	Calls  []string // method and path of every request that had the right key
}

func New(instanceName, apiKey string) *Server {
	s := &Server{InstanceName: instanceName, APIKey: apiKey, nextID: 1}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// Notifications returns the stored connections.
func (s *Server) Notifications() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.list...)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Api-Key") != s.APIKey {
		http.Error(w, `{"message":"Unauthorized"}`, http.StatusUnauthorized)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls = append(s.Calls, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v3/system/status":
		json.NewEncoder(w).Encode(map[string]any{"instanceName": s.InstanceName, "appName": "Radarr"})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v3/notification":
		json.NewEncoder(w).Encode(s.list)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v3/notification":
		var n map[string]any
		if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		n = s.store(n)
		if !s.IgnoreWrites {
			s.list = append(s.list, n)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(n)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v3/notification/"):
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/v3/notification/"))
		var n map[string]any
		if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		for i, have := range s.list {
			if int(have["id"].(float64)) == id {
				n["id"] = float64(id)
				n = s.store(n)
				if !s.IgnoreWrites {
					s.list[i] = n
				}
				w.WriteHeader(http.StatusAccepted)
				json.NewEncoder(w).Encode(n)
				return
			}
		}
		http.Error(w, `{"message":"NotFound"}`, http.StatusNotFound)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v3/notification/test":
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, `{"message":"NotFound"}`, http.StatusNotFound)
	}
}

// store does what the real app does to what it is given: assigns an id,
// hides the password, and round-trips through JSON so numbers are float64.
func (s *Server) store(n map[string]any) map[string]any {
	if _, ok := n["id"]; !ok {
		n["id"] = float64(s.nextID)
		s.nextID++
	}
	if fields, ok := n["fields"].([]any); ok {
		for _, f := range fields {
			if m, ok := f.(map[string]any); ok && m["name"] == "password" {
				delete(m, "value")
			}
		}
	}
	b, _ := json.Marshal(n)
	var out map[string]any
	json.Unmarshal(b, &out)
	return out
}
