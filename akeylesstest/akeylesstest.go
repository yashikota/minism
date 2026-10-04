// Package akeylesstest gives tests a real akeyless-go V2ApiService whose HTTP
// client is an in-memory Akeyless gateway.
//
// Akeyless publishes no server implementation, so the state is ours. It covers
// static secrets: create, get-secret-value, update-secret-val (new version),
// delete-item, list-items, describe-item and auth. Everything else answers 501.
package akeylesstest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/akeylesslabs/akeyless-go/v5"

	"github.com/yashikota/minism/internal/rtfake"
)

type version struct {
	value   string
	created time.Time
}

type item struct {
	id               int64
	name             string
	desc             string
	tags             []string
	versions         []*version
	created, updated time.Time
}

// Server is an in-memory Akeyless gateway.
type Server struct {
	t     testing.TB
	mu    sync.Mutex
	items map[string]*item // by normalized name
	seq   int64
	now   func() time.Time
	mux   *http.ServeMux
}

// New returns an empty gateway.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{t: t, items: map[string]*item{}, now: func() time.Time { return time.Now().UTC() }}
	m := http.NewServeMux()
	m.HandleFunc("POST /auth", s.auth)
	m.HandleFunc("POST /create-secret", s.createSecret)
	m.HandleFunc("POST /get-secret-value", s.getSecretValue)
	m.HandleFunc("POST /update-secret-val", s.updateSecretVal)
	m.HandleFunc("POST /delete-item", s.deleteItem)
	m.HandleFunc("POST /list-items", s.listItems)
	m.HandleFunc("POST /describe-item", s.describeItem)
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, http.StatusNotImplemented, fmt.Sprintf("minism: akeyless %s not implemented", r.URL.Path))
	})
	s.mux = m
	return s
}

// Client returns a real Akeyless API client talking to this gateway.
func (s *Server) Client() *akeyless.V2ApiService {
	cfg := akeyless.NewConfiguration()
	cfg.Servers = akeyless.ServerConfigurations{{URL: "https://akeyless.invalid"}}
	cfg.HTTPClient = rtfake.Client(s.mux)
	return akeyless.NewAPIClient(cfg).V2Api
}

// Value returns the latest value of a secret (fixture inspection).
func (s *Server) Value(name string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[norm(name)]
	if !ok {
		return "", false
	}
	return it.versions[len(it.versions)-1].value, true
}

func norm(name string) string { return "/" + strings.TrimLeft(name, "/") }

func fail(w http.ResponseWriter, status int, msg string) {
	rtfake.JSON(w, status, "", map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request) (map[string]json.RawMessage, bool) {
	var in map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return nil, false
	}
	return in, true
}

func field[T any](in map[string]json.RawMessage, key string) (v T, ok bool) {
	raw, present := in[key]
	if !present || string(raw) == "null" {
		return v, false
	}
	return v, json.Unmarshal(raw, &v) == nil
}

func (s *Server) auth(w http.ResponseWriter, _ *http.Request) {
	rtfake.JSON(w, http.StatusOK, "", map[string]string{
		"token": "t-minism", "expiration": s.now().Add(time.Hour).Format(time.RFC3339),
	})
}

func (s *Server) createSecret(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r)
	if !ok {
		return
	}
	name, _ := field[string](in, "name")
	value, _ := field[string](in, "value")
	if name == "" {
		fail(w, http.StatusBadRequest, "name is required")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := norm(name)
	if _, exists := s.items[key]; exists {
		fail(w, http.StatusBadRequest, fmt.Sprintf("Item '%s' already exists", key))
		return
	}
	now := s.now()
	s.seq++
	it := &item{id: s.seq, name: key, created: now, updated: now, versions: []*version{{value: value, created: now}}}
	it.desc, _ = field[string](in, "description")
	it.tags, _ = field[[]string](in, "tags")
	s.items[key] = it
	rtfake.JSON(w, http.StatusOK, "", map[string]string{"name": key})
}

func (s *Server) getSecretValue(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r)
	if !ok {
		return
	}
	names, _ := field[[]string](in, "names")
	ver, _ := field[int](in, "version")
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]any{}
	for _, n := range names {
		it, ok := s.items[norm(n)]
		if !ok {
			fail(w, http.StatusNotFound, fmt.Sprintf("Item '%s' not found", norm(n)))
			return
		}
		v := it.versions[len(it.versions)-1]
		if ver > 0 {
			if ver > len(it.versions) {
				fail(w, http.StatusNotFound, fmt.Sprintf("Version %d of '%s' not found", ver, norm(n)))
				return
			}
			v = it.versions[ver-1]
		}
		out[n] = v.value
	}
	rtfake.JSON(w, http.StatusOK, "", out)
}

func (s *Server) updateSecretVal(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r)
	if !ok {
		return
	}
	name, _ := field[string](in, "name")
	value, _ := field[string](in, "value")
	keepPrev, _ := field[string](in, "keep-prev-version")
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[norm(name)]
	if !ok {
		fail(w, http.StatusNotFound, fmt.Sprintf("Item '%s' not found", norm(name)))
		return
	}
	now := s.now()
	if keepPrev == "false" {
		it.versions[len(it.versions)-1] = &version{value: value, created: now}
	} else {
		it.versions = append(it.versions, &version{value: value, created: now})
	}
	it.updated = now
	rtfake.JSON(w, http.StatusOK, "", map[string]string{"name": it.name})
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r)
	if !ok {
		return
	}
	name, _ := field[string](in, "name")
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[norm(name)]
	if !ok {
		fail(w, http.StatusNotFound, fmt.Sprintf("Item '%s' not found", norm(name)))
		return
	}
	delete(s.items, it.name)
	rtfake.JSON(w, http.StatusOK, "", map[string]any{
		"item_name": it.name, "item_id": it.id, "deletion_date": s.now().Format(time.RFC3339), "version_deleted": len(it.versions),
	})
}

func (it *item) describe() map[string]any {
	vs := make([]map[string]any, 0, len(it.versions))
	for i, v := range it.versions {
		vs = append(vs, map[string]any{"version": i + 1, "creation_date": v.created.Format(time.RFC3339), "modification_date": v.created.Format(time.RFC3339)})
	}
	return map[string]any{
		"item_id": it.id, "item_name": it.name, "item_type": "STATIC_SECRET", "item_sub_type": "generic",
		"item_metadata": it.desc, "item_tags": it.tags, "last_version": len(it.versions), "item_versions": vs,
		"creation_date": it.created.Format(time.RFC3339), "modification_date": it.updated.Format(time.RFC3339),
		"is_enabled": true, "item_state": "Enabled",
	}
}

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r)
	if !ok {
		return
	}
	path, _ := field[string](in, "path")
	prefix := strings.TrimRight(norm(path), "/") + "/"
	s.mu.Lock()
	defer s.mu.Unlock()
	var names []string
	for n := range s.items {
		if strings.HasPrefix(n, prefix) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	items := []map[string]any{}
	for _, n := range names {
		items = append(items, s.items[n].describe())
	}
	rtfake.JSON(w, http.StatusOK, "", map[string]any{"items": items, "has_next": false})
}

func (s *Server) describeItem(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r)
	if !ok {
		return
	}
	name, _ := field[string](in, "name")
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.items[norm(name)]
	if !ok {
		fail(w, http.StatusNotFound, fmt.Sprintf("Item '%s' not found", norm(name)))
		return
	}
	rtfake.JSON(w, http.StatusOK, "", it.describe())
}
