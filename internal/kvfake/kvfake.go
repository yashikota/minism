// Package kvfake is an in-memory implementation of the Vault-compatible HTTP API
// subset that tests use: sys/mounts, and the KV secrets engine in both v1 and v2.
//
// Vault and OpenBao expose the same wire protocol, so vaulttest and openbaotest
// share this handler and differ only in which official api client they return.
// It is deliberately not a Vault: no auth, policies, leases or seal. Any token is
// accepted. Unsupported paths answer 501.
package kvfake

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type version struct {
	data      map[string]any
	created   time.Time
	deletedAt *time.Time
	destroyed bool
}

type entry struct {
	versions []*version // v2
	created  time.Time
	updated  time.Time
	custom   map[string]string
	v1       map[string]any // v1
}

type mount struct {
	version int
	entries map[string]*entry
}

// Server is the http.Handler. The zero value is not usable; call New.
type Server struct {
	mu     sync.Mutex
	mounts map[string]*mount // key without trailing slash
	now    func() time.Time
}

// New returns a Server with a kv-v2 engine mounted at "secret/", like `vault server -dev`.
func New() *Server {
	s := &Server{mounts: map[string]*mount{}, now: func() time.Time { return time.Now().UTC() }}
	s.mounts["secret"] = &mount{version: 2, entries: map[string]*entry{}}
	return s
}

func ts(t time.Time) string { return t.Format(time.RFC3339Nano) }

func reply(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"request_id": "minism", "lease_id": "", "renewable": false, "lease_duration": 0,
		"data": data, "wrap_info": nil, "warnings": nil, "auth": nil,
	})
}

func fail(w http.ResponseWriter, status int, msgs ...string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if msgs == nil {
		msgs = []string{}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"errors": msgs})
}

func notFound(w http.ResponseWriter) { fail(w, http.StatusNotFound) }

func decode(r *http.Request) map[string]any {
	var in map[string]any
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in == nil {
		in = map[string]any{}
	}
	return in
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/v1/")
	if p == r.URL.Path {
		notFound(w)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if p == "sys/mounts" || strings.HasPrefix(p, "sys/mounts/") {
		s.sysMounts(w, r, strings.Trim(strings.TrimPrefix(p, "sys/mounts"), "/"))
		return
	}
	name, m, rest := s.resolve(p)
	if m == nil {
		notFound(w)
		return
	}
	list := r.Method == "LIST" || r.URL.Query().Get("list") == "true"
	if m.version == 1 {
		s.kv1(w, r, m, rest, list)
	} else {
		s.kv2(w, r, m, name, rest, list)
	}
}

// resolve finds the mount owning p (longest prefix wins).
func (s *Server) resolve(p string) (string, *mount, string) {
	best := ""
	for name := range s.mounts {
		if (p == name || strings.HasPrefix(p, name+"/")) && len(name) > len(best) {
			best = name
		}
	}
	if best == "" {
		return "", nil, ""
	}
	return best, s.mounts[best], strings.TrimPrefix(strings.TrimPrefix(p, best), "/")
}

// ---- sys/mounts ----

func (s *Server) sysMounts(w http.ResponseWriter, r *http.Request, path string) {
	switch {
	case path == "" && r.Method == http.MethodGet:
		out := map[string]any{}
		for n, m := range s.mounts {
			out[n+"/"] = map[string]any{"type": "kv", "options": map[string]string{"version": strconv.Itoa(m.version)}}
		}
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{"data": out, "request_id": "minism"}
		for k, v := range out {
			body[k] = v // older clients read mounts from the top level
		}
		_ = json.NewEncoder(w).Encode(body)
	case path != "" && (r.Method == http.MethodPost || r.Method == http.MethodPut):
		in := decode(r)
		if t, _ := in["type"].(string); t != "kv" && t != "kv-v2" && t != "generic" {
			fail(w, http.StatusBadRequest, fmt.Sprintf("minism: mount type %q not implemented", t))
			return
		}
		if _, exists := s.mounts[path]; exists {
			fail(w, http.StatusBadRequest, "path is already in use at "+path+"/")
			return
		}
		v := 1
		if opts, ok := in["options"].(map[string]any); ok && opts["version"] == "2" {
			v = 2
		}
		if in["type"] == "kv-v2" {
			v = 2
		}
		s.mounts[path] = &mount{version: v, entries: map[string]*entry{}}
		reply(w, http.StatusNoContent, nil)
	case path != "" && r.Method == http.MethodDelete:
		delete(s.mounts, path)
		reply(w, http.StatusNoContent, nil)
	default:
		fail(w, http.StatusNotImplemented, "minism: sys/mounts operation not implemented")
	}
}

// ---- kv v1 ----

func (s *Server) kv1(w http.ResponseWriter, r *http.Request, m *mount, path string, list bool) {
	path = strings.Trim(path, "/")
	switch {
	case list:
		keys := m.list(strings.Trim(r.URL.Path, "/"), path)
		if len(keys) == 0 {
			notFound(w)
			return
		}
		reply(w, http.StatusOK, map[string]any{"keys": keys})
	case r.Method == http.MethodGet:
		e, ok := m.entries[path]
		if !ok {
			notFound(w)
			return
		}
		reply(w, http.StatusOK, e.v1)
	case r.Method == http.MethodPost || r.Method == http.MethodPut:
		m.entries[path] = &entry{v1: decode(r)}
		reply(w, http.StatusNoContent, nil)
	case r.Method == http.MethodDelete:
		delete(m.entries, path)
		reply(w, http.StatusNoContent, nil)
	default:
		fail(w, http.StatusNotImplemented, "minism: kv-v1 operation not implemented")
	}
}

// list returns the immediate children under dir; sub-directories end in "/".
func (m *mount) list(_, dir string) []string {
	prefix := ""
	if dir != "" {
		prefix = dir + "/"
	}
	seen := map[string]bool{}
	for k, e := range m.entries {
		if !strings.HasPrefix(k, prefix) || (e.v1 == nil && e.live() == nil) {
			continue
		}
		child := strings.TrimPrefix(k, prefix)
		if i := strings.IndexByte(child, '/'); i >= 0 {
			child = child[:i+1]
		}
		seen[child] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ---- kv v2 ----

// live is the newest version that has not been deleted or destroyed, or nil.
func (e *entry) live() *version {
	if len(e.versions) == 0 {
		return nil
	}
	return e.versions[len(e.versions)-1]
}

func (v *version) meta(n int) map[string]any {
	del := ""
	if v.deletedAt != nil {
		del = ts(*v.deletedAt)
	}
	return map[string]any{"version": n, "created_time": ts(v.created), "deletion_time": del, "destroyed": v.destroyed, "custom_metadata": nil}
}

func (s *Server) kv2(w http.ResponseWriter, r *http.Request, m *mount, _ string, rest string, list bool) {
	op, path, _ := strings.Cut(rest, "/")
	path = strings.Trim(path, "/")
	switch op {
	case "data":
		s.kv2Data(w, r, m, path)
	case "metadata":
		s.kv2Metadata(w, r, m, path, list)
	case "delete", "undelete", "destroy":
		s.kv2Versions(w, r, m, op, path)
	default:
		fail(w, http.StatusNotImplemented, "minism: kv-v2 path "+op+" not implemented")
	}
}

func (s *Server) kv2Data(w http.ResponseWriter, r *http.Request, m *mount, path string) {
	e := m.entries[path]
	switch r.Method {
	case http.MethodGet:
		if e == nil || len(e.versions) == 0 {
			notFound(w)
			return
		}
		n := len(e.versions)
		if q := r.URL.Query().Get("version"); q != "" && q != "0" {
			n, _ = strconv.Atoi(q)
		}
		if n < 1 || n > len(e.versions) {
			notFound(w)
			return
		}
		v := e.versions[n-1]
		if v.deletedAt != nil || v.destroyed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": nil, "metadata": v.meta(n)}})
			return
		}
		reply(w, http.StatusOK, map[string]any{"data": v.data, "metadata": v.meta(n)})
	case http.MethodPost, http.MethodPut:
		in := decode(r)
		data, _ := in["data"].(map[string]any)
		if data == nil {
			fail(w, http.StatusBadRequest, "no data provided")
			return
		}
		if opts, ok := in["options"].(map[string]any); ok {
			if cas, ok := opts["cas"].(float64); ok {
				cur := 0
				if e != nil {
					cur = len(e.versions)
				}
				if int(cas) != cur {
					fail(w, http.StatusBadRequest, "check-and-set parameter did not match the current version")
					return
				}
			}
		}
		now := s.now()
		if e == nil {
			e = &entry{created: now}
			m.entries[path] = e
		}
		v := &version{data: data, created: now}
		e.versions = append(e.versions, v)
		e.updated = now
		reply(w, http.StatusOK, v.meta(len(e.versions)))
	case http.MethodDelete:
		if e != nil && len(e.versions) > 0 {
			now := s.now()
			e.versions[len(e.versions)-1].deletedAt = &now
		}
		reply(w, http.StatusNoContent, nil)
	default:
		fail(w, http.StatusNotImplemented, "minism: kv-v2 data operation not implemented")
	}
}

func (s *Server) kv2Metadata(w http.ResponseWriter, r *http.Request, m *mount, path string, list bool) {
	if list {
		keys := m.list("", path)
		if len(keys) == 0 {
			notFound(w)
			return
		}
		reply(w, http.StatusOK, map[string]any{"keys": keys})
		return
	}
	e := m.entries[path]
	switch r.Method {
	case http.MethodGet:
		if e == nil {
			notFound(w)
			return
		}
		vs := map[string]any{}
		oldest := 0
		for i, v := range e.versions {
			vs[strconv.Itoa(i+1)] = v.meta(i + 1)
			if oldest == 0 && !v.destroyed {
				oldest = i + 1
			}
		}
		reply(w, http.StatusOK, map[string]any{
			"current_version": len(e.versions), "oldest_version": oldest, "max_versions": 0,
			"created_time": ts(e.created), "updated_time": ts(e.updated), "cas_required": false,
			"delete_version_after": "0s", "custom_metadata": e.custom, "versions": vs,
		})
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		if e == nil {
			now := s.now()
			e = &entry{created: now, updated: now}
			m.entries[path] = e
		}
		if cm, ok := decode(r)["custom_metadata"].(map[string]any); ok {
			e.custom = map[string]string{}
			for k, v := range cm {
				e.custom[k], _ = v.(string)
			}
		}
		reply(w, http.StatusNoContent, nil)
	case http.MethodDelete:
		delete(m.entries, path)
		reply(w, http.StatusNoContent, nil)
	default:
		fail(w, http.StatusNotImplemented, "minism: kv-v2 metadata operation not implemented")
	}
}

func (s *Server) kv2Versions(w http.ResponseWriter, r *http.Request, m *mount, op, path string) {
	e := m.entries[path]
	if e == nil {
		notFound(w)
		return
	}
	var nums []int
	if raw, ok := decode(r)["versions"].([]any); ok {
		for _, n := range raw {
			if f, ok := n.(float64); ok {
				nums = append(nums, int(f))
			}
		}
	}
	if op == "delete" && r.Method == http.MethodDelete { // DELETE /delete/... is not a Vault route
		fail(w, http.StatusMethodNotAllowed)
		return
	}
	now := s.now()
	for _, n := range nums {
		if n < 1 || n > len(e.versions) {
			continue
		}
		v := e.versions[n-1]
		switch op {
		case "delete":
			if !v.destroyed {
				v.deletedAt = &now
			}
		case "undelete":
			v.deletedAt = nil
		case "destroy":
			v.destroyed, v.data = true, nil
		}
	}
	reply(w, http.StatusNoContent, nil)
}
