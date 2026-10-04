// Package ibmsmtest gives tests a real *secretsmanagerv2.SecretsManagerV2
// whose HTTP client is an in-memory IBM Cloud Secrets Manager.
//
// IBM publishes no server implementation, so the state is ours. It covers
// arbitrary secrets: create/get/list/delete, metadata patch, versions and the
// by-name lookup. Other secret types and operations answer 501.
package ibmsmtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/secrets-manager-go-sdk/v2/secretsmanagerv2"

	"github.com/yashikota/minism/internal/rtfake"
)

const (
	serviceURL   = "https://minism.us-south.secrets-manager.appdomain.cloud"
	defaultGroup = "default"
)

type version struct {
	id      string
	payload string
	created time.Time
	meta    map[string]any
}

type secret struct {
	id, name, group, desc string
	labels                []string
	custom                map[string]any
	created, updated      time.Time
	versions              []*version
}

// Server is an in-memory Secrets Manager instance.
type Server struct {
	t       testing.TB
	mu      sync.Mutex
	secrets map[string]*secret // by id
	seq     int
	now     func() time.Time
	mux     *http.ServeMux
}

// New returns an empty instance.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{t: t, secrets: map[string]*secret{}, now: func() time.Time { return time.Now().UTC() }}
	m := http.NewServeMux()
	m.HandleFunc("POST /api/v2/secrets", s.create)
	m.HandleFunc("GET /api/v2/secrets", s.list)
	m.HandleFunc("GET /api/v2/secrets/{id}", s.get)
	m.HandleFunc("DELETE /api/v2/secrets/{id}", s.delete)
	m.HandleFunc("GET /api/v2/secrets/{id}/metadata", s.getMetadata)
	m.HandleFunc("PATCH /api/v2/secrets/{id}/metadata", s.patchMetadata)
	m.HandleFunc("POST /api/v2/secrets/{id}/versions", s.createVersion)
	m.HandleFunc("GET /api/v2/secrets/{id}/versions", s.listVersions)
	m.HandleFunc("GET /api/v2/secrets/{id}/versions/{vid}", s.getVersion)
	m.HandleFunc("GET /api/v2/secret_groups/{group}/secret_types/{typ}/secrets/{name}", s.byName)
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, http.StatusNotImplemented, "not_implemented", fmt.Sprintf("minism: ibmsm %s %s not implemented", r.Method, r.URL.Path))
	})
	s.mux = m
	return s
}

// Client returns a real SDK client wired to this instance.
func (s *Server) Client() *secretsmanagerv2.SecretsManagerV2 {
	s.t.Helper()
	c, err := secretsmanagerv2.NewSecretsManagerV2(&secretsmanagerv2.SecretsManagerV2Options{
		URL: serviceURL, Authenticator: &core.NoAuthAuthenticator{},
	})
	if err != nil {
		s.t.Fatalf("ibmsmtest: new client: %v", err)
	}
	c.Service.SetHTTPClient(rtfake.Client(s.mux))
	return c
}

// Payload returns the current payload of a secret by name (fixture inspection).
func (s *Server) Payload(name string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sec := range s.secrets {
		if sec.name == name {
			return sec.versions[len(sec.versions)-1].payload, true
		}
	}
	return "", false
}

func fail(w http.ResponseWriter, status int, code, msg string) {
	rtfake.JSON(w, status, "", map[string]any{
		"errors":      []map[string]string{{"code": code, "message": msg, "more_info": ""}},
		"trace":       "minism",
		"status_code": status,
	})
}

func notFound(w http.ResponseWriter, what string) {
	fail(w, http.StatusNotFound, "not_found", what+" not found")
}

func ts(t time.Time) string { return t.Format(time.RFC3339Nano) }

func (s *Server) newID() string {
	s.seq++
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", s.seq)
}

func (sec *secret) metadata() map[string]any {
	last := sec.versions[len(sec.versions)-1]
	_ = last
	return map[string]any{
		"id": sec.id, "name": sec.name, "secret_type": "arbitrary", "secret_group_id": sec.group,
		"crn":         "crn:v1:bluemix:public:secrets-manager:us-south:a/minism:instance::secret:" + sec.id,
		"description": sec.desc, "labels": nonNil(sec.labels), "custom_metadata": nonNilMap(sec.custom),
		"state": 1, "state_description": "active", "versions_total": len(sec.versions), "locks_total": 0,
		"downloaded": false, "created_by": "iam-minism", "created_at": ts(sec.created), "updated_at": ts(sec.updated),
	}
}

func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func (sec *secret) full() map[string]any {
	m := sec.metadata()
	m["payload"] = sec.versions[len(sec.versions)-1].payload
	return m
}

func (v *version) metadata(sec *secret, withPayload bool) map[string]any {
	m := map[string]any{
		"id": v.id, "secret_id": sec.id, "secret_name": sec.name, "secret_type": "arbitrary",
		"secret_group_id": sec.group, "payload_available": true, "downloaded": false, "auto_rotated": false,
		"created_by": "iam-minism", "created_at": ts(v.created), "version_custom_metadata": nonNilMap(v.meta),
	}
	if withPayload {
		m["payload"] = v.payload
	}
	return m
}

func decode(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	var in map[string]any
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "invalid_body", err.Error())
		return nil, false
	}
	return in, true
}

func str(m map[string]any, k string) string { v, _ := m[k].(string); return v }

func labels(m map[string]any) []string {
	var out []string
	if raw, ok := m["labels"].([]any); ok {
		for _, l := range raw {
			if s, ok := l.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r)
	if !ok {
		return
	}
	if typ := str(in, "secret_type"); typ != "arbitrary" {
		fail(w, http.StatusNotImplemented, "not_implemented", fmt.Sprintf("minism: ibmsm secret type %q not implemented", typ))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	name, group := str(in, "name"), str(in, "secret_group_id")
	if group == "" {
		group = defaultGroup
	}
	for _, sec := range s.secrets {
		if sec.name == name && sec.group == group {
			fail(w, http.StatusConflict, "conflict", "A secret with the same name already exists in this group")
			return
		}
	}
	now := s.now()
	custom, _ := in["custom_metadata"].(map[string]any)
	sec := &secret{
		id: s.newID(), name: name, group: group, desc: str(in, "description"), labels: labels(in),
		custom: custom, created: now, updated: now,
	}
	sec.versions = []*version{{id: s.newID(), payload: str(in, "payload"), created: now}}
	s.secrets[sec.id] = sec
	rtfake.JSON(w, http.StatusCreated, "", sec.full())
}

func (s *Server) find(w http.ResponseWriter, r *http.Request) (*secret, bool) {
	sec, ok := s.secrets[r.PathValue("id")]
	if !ok {
		notFound(w, "Secret")
	}
	return sec, ok
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sec, ok := s.find(w, r); ok {
		rtfake.JSON(w, http.StatusOK, "", sec.full())
	}
}

func (s *Server) getMetadata(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sec, ok := s.find(w, r); ok {
		rtfake.JSON(w, http.StatusOK, "", sec.metadata())
	}
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sec, ok := s.find(w, r); ok {
		delete(s.secrets, sec.id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) byName(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sec := range s.secrets {
		if sec.name == r.PathValue("name") && sec.group == r.PathValue("group") && r.PathValue("typ") == "arbitrary" {
			rtfake.JSON(w, http.StatusOK, "", sec.full())
			return
		}
	}
	notFound(w, "Secret")
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit <= 0 {
		limit = 200
	}
	search := q.Get("search")
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []*secret
	for _, sec := range s.secrets {
		if search == "" || strings.Contains(sec.name, search) {
			all = append(all, sec)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].name < all[j].name })
	page := []map[string]any{}
	for i := offset; i < len(all) && i < offset+limit; i++ {
		page = append(page, all[i].metadata())
	}
	link := func(off int) map[string]string {
		return map[string]string{"href": fmt.Sprintf("%s/api/v2/secrets?offset=%d&limit=%d", serviceURL, off, limit)}
	}
	resp := map[string]any{"total_count": len(all), "limit": limit, "offset": offset, "first": link(0), "last": link(0), "secrets": page}
	if offset+limit < len(all) {
		resp["next"] = link(offset + limit)
	}
	rtfake.JSON(w, http.StatusOK, "", resp)
}

func (s *Server) patchMetadata(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.find(w, r)
	if !ok {
		return
	}
	if v, ok := in["name"].(string); ok {
		sec.name = v
	}
	if v, ok := in["description"].(string); ok {
		sec.desc = v
	}
	if _, ok := in["labels"]; ok {
		sec.labels = labels(in)
	}
	if v, ok := in["custom_metadata"].(map[string]any); ok {
		sec.custom = v
	}
	sec.updated = s.now()
	rtfake.JSON(w, http.StatusOK, "", sec.metadata())
}

func (s *Server) createVersion(w http.ResponseWriter, r *http.Request) {
	in, ok := decode(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.find(w, r)
	if !ok {
		return
	}
	meta, _ := in["version_custom_metadata"].(map[string]any)
	v := &version{id: s.newID(), payload: str(in, "payload"), created: s.now(), meta: meta}
	sec.versions = append(sec.versions, v)
	sec.updated = v.created
	rtfake.JSON(w, http.StatusCreated, "", v.metadata(sec, true))
}

func (s *Server) listVersions(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.find(w, r)
	if !ok {
		return
	}
	out := []map[string]any{}
	for i := len(sec.versions) - 1; i >= 0; i-- { // newest first
		out = append(out, sec.versions[i].metadata(sec, false))
	}
	rtfake.JSON(w, http.StatusOK, "", map[string]any{"versions": out, "total_count": len(out)})
}

func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.find(w, r)
	if !ok {
		return
	}
	vid := r.PathValue("vid")
	switch vid {
	case "current":
		rtfake.JSON(w, http.StatusOK, "", sec.versions[len(sec.versions)-1].metadata(sec, true))
		return
	case "previous":
		if len(sec.versions) > 1 {
			rtfake.JSON(w, http.StatusOK, "", sec.versions[len(sec.versions)-2].metadata(sec, true))
			return
		}
	default:
		for _, v := range sec.versions {
			if v.id == vid {
				rtfake.JSON(w, http.StatusOK, "", v.metadata(sec, true))
				return
			}
		}
	}
	notFound(w, "Secret version")
}
