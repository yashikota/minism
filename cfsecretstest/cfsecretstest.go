// Package cfsecretstest gives tests a real *cloudflare.Client whose HTTP client
// is an in-memory Cloudflare Secrets Store.
//
// Cloudflare publishes no server implementation, so the state is ours. Like the
// real API, secret values are write-only through the client; read them back with
// Server.Value, which is test-fixture inspection and not part of the provider
// contract. Bulk delete answers 501.
package cfsecretstest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/option"

	"github.com/yashikota/minism/internal/rtfake"
)

const (
	// AccountID is the account every request must use.
	AccountID = "00000000000000000000000000000001"
	apiBase   = "https://cloudflare.invalid/client/v4/"
	prefix    = "/client/v4/accounts/{account}/secrets_store/stores"
)

type secret struct {
	id, name, comment string
	value             string
	scopes            []string
	created, modified time.Time
}

type store struct {
	id, name string
	created  time.Time
	secrets  map[string]*secret // by id
}

// Server is an in-memory Secrets Store.
type Server struct {
	t      testing.TB
	mu     sync.Mutex
	stores map[string]*store
	seq    int
	now    func() time.Time
	mux    *http.ServeMux
	def    *store
}

// New returns a Server with one empty store named "default"; see StoreID.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{t: t, stores: map[string]*store{}, now: func() time.Time { return time.Now().UTC() }}
	s.def = s.newStore("default")
	m := http.NewServeMux()
	m.HandleFunc("POST "+prefix, s.createStore)
	m.HandleFunc("GET "+prefix, s.listStores)
	m.HandleFunc("DELETE "+prefix+"/{store}", s.deleteStore)
	m.HandleFunc("POST "+prefix+"/{store}/secrets", s.createSecrets)
	m.HandleFunc("GET "+prefix+"/{store}/secrets", s.listSecrets)
	m.HandleFunc("GET "+prefix+"/{store}/secrets/{secret}", s.getSecret)
	m.HandleFunc("PATCH "+prefix+"/{store}/secrets/{secret}", s.editSecret)
	m.HandleFunc("DELETE "+prefix+"/{store}/secrets/{secret}", s.deleteSecret)
	m.HandleFunc("POST "+prefix+"/{store}/secrets/{secret}/duplicate", s.duplicateSecret)
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, http.StatusNotImplemented, 10000, fmt.Sprintf("minism: cloudflare %s %s not implemented", r.Method, r.URL.Path))
	})
	s.mux = m
	return s
}

// Client returns a real Cloudflare client talking to this Server.
func (s *Server) Client() *cloudflare.Client {
	return cloudflare.NewClient(
		option.WithAPIToken("minism"),
		option.WithBaseURL(apiBase),
		option.WithHTTPClient(rtfake.Client(s.mux)),
		option.WithMaxRetries(0),
	)
}

// StoreID is the ID of the default store.
func (s *Server) StoreID() string { return s.def.id }

// Value reads a secret's value by store and name (fixture inspection only).
func (s *Server) Value(storeID, name string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.stores[storeID]; ok {
		for _, sec := range st.secrets {
			if sec.name == name {
				return sec.value, true
			}
		}
	}
	return "", false
}

func (s *Server) newID() string {
	s.seq++
	return fmt.Sprintf("%032x", s.seq)
}

func (s *Server) newStore(name string) *store {
	st := &store{id: s.newID(), name: name, created: s.now(), secrets: map[string]*secret{}}
	s.stores[st.id] = st
	return st
}

func ts(t time.Time) string { return t.Format(time.RFC3339Nano) }

func envelope(result any, info any) map[string]any {
	e := map[string]any{"success": true, "errors": []any{}, "messages": []any{}, "result": result}
	if info != nil {
		e["result_info"] = info
	}
	return e
}

func fail(w http.ResponseWriter, status, code int, msg string) {
	rtfake.JSON(w, status, "", map[string]any{
		"success": false, "errors": []map[string]any{{"code": code, "message": msg}}, "messages": []any{}, "result": nil,
	})
}

// accountOK rejects requests for any account other than AccountID.
func accountOK(w http.ResponseWriter, r *http.Request) bool {
	if r.PathValue("account") != AccountID {
		fail(w, http.StatusForbidden, 10000, "Authentication error")
		return false
	}
	return true
}

func (st *store) json(account string) map[string]any {
	return map[string]any{"id": st.id, "name": st.name, "account_id": account, "created": ts(st.created), "modified": ts(st.created)}
}

func (sec *secret) json(storeID string) map[string]any {
	scopes := sec.scopes
	if scopes == nil {
		scopes = []string{}
	}
	return map[string]any{
		"id": sec.id, "name": sec.name, "store_id": storeID, "status": "active", "comment": sec.comment,
		"scopes": scopes, "created": ts(sec.created), "modified": ts(sec.modified),
	}
}

func (s *Server) createStore(w http.ResponseWriter, r *http.Request) {
	if !accountOK(w, r) {
		return
	}
	var in struct{ Name string }
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.newStore(in.Name)
	rtfake.JSON(w, http.StatusOK, "", envelope(st.json(AccountID), nil))
}

func page(r *http.Request, n int) (from, to, perPage, pg int) {
	pg, _ = strconv.Atoi(r.URL.Query().Get("page"))
	if pg < 1 {
		pg = 1
	}
	perPage, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
	if perPage < 1 {
		perPage = 20
	}
	from = min((pg-1)*perPage, n)
	to = min(from+perPage, n)
	return
}

func info(pg, perPage, count, total int) map[string]int {
	return map[string]int{"page": pg, "per_page": perPage, "count": count, "total_count": total, "total_pages": (total + perPage - 1) / perPage}
}

func (s *Server) listStores(w http.ResponseWriter, r *http.Request) {
	if !accountOK(w, r) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []*store
	for _, st := range s.stores {
		all = append(all, st)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].id < all[j].id })
	from, to, perPage, pg := page(r, len(all))
	out := []map[string]any{}
	for _, st := range all[from:to] {
		out = append(out, st.json(AccountID))
	}
	rtfake.JSON(w, http.StatusOK, "", envelope(out, info(pg, perPage, len(out), len(all))))
}

func (s *Server) store(w http.ResponseWriter, r *http.Request) (*store, bool) {
	if !accountOK(w, r) {
		return nil, false
	}
	st, ok := s.stores[r.PathValue("store")]
	if !ok {
		fail(w, http.StatusNotFound, 10000, "store not found")
	}
	return st, ok
}

func (s *Server) deleteStore(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.store(w, r); ok {
		delete(s.stores, st.id)
		rtfake.JSON(w, http.StatusOK, "", envelope(st.json(AccountID), nil))
	}
}

func (st *store) byName(name string) *secret {
	for _, sec := range st.secrets {
		if sec.name == name {
			return sec
		}
	}
	return nil
}

func (s *Server) createSecrets(w http.ResponseWriter, r *http.Request) {
	var in []struct {
		Name    string   `json:"name"`
		Value   string   `json:"value"`
		Scopes  []string `json:"scopes"`
		Comment string   `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, 10000, "invalid body: "+err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.store(w, r)
	if !ok {
		return
	}
	for _, p := range in {
		if st.byName(p.Name) != nil {
			fail(w, http.StatusConflict, 10000, fmt.Sprintf("secret %q already exists", p.Name))
			return
		}
	}
	out := []map[string]any{}
	for _, p := range in {
		now := s.now()
		sec := &secret{id: s.newID(), name: p.Name, value: p.Value, scopes: p.Scopes, comment: p.Comment, created: now, modified: now}
		st.secrets[sec.id] = sec
		out = append(out, sec.json(st.id))
	}
	rtfake.JSON(w, http.StatusOK, "", envelope(out, nil))
}

func (s *Server) listSecrets(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.store(w, r)
	if !ok {
		return
	}
	var all []*secret
	for _, sec := range st.secrets {
		all = append(all, sec)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].name < all[j].name })
	from, to, perPage, pg := page(r, len(all))
	out := []map[string]any{}
	for _, sec := range all[from:to] {
		out = append(out, sec.json(st.id))
	}
	rtfake.JSON(w, http.StatusOK, "", envelope(out, info(pg, perPage, len(out), len(all))))
}

func (s *Server) secret(w http.ResponseWriter, r *http.Request) (*store, *secret, bool) {
	st, ok := s.store(w, r)
	if !ok {
		return nil, nil, false
	}
	sec, ok := st.secrets[r.PathValue("secret")]
	if !ok {
		fail(w, http.StatusNotFound, 10000, "secret not found")
	}
	return st, sec, ok
}

func (s *Server) getSecret(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, sec, ok := s.secret(w, r); ok {
		rtfake.JSON(w, http.StatusOK, "", envelope(sec.json(st.id), nil))
	}
}

func (s *Server) editSecret(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Value   *string  `json:"value"`
		Scopes  []string `json:"scopes"`
		Comment *string  `json:"comment"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.mu.Lock()
	defer s.mu.Unlock()
	st, sec, ok := s.secret(w, r)
	if !ok {
		return
	}
	if in.Value != nil {
		sec.value = *in.Value
	}
	if in.Scopes != nil {
		sec.scopes = in.Scopes
	}
	if in.Comment != nil {
		sec.comment = *in.Comment
	}
	sec.modified = s.now()
	rtfake.JSON(w, http.StatusOK, "", envelope(sec.json(st.id), nil))
}

func (s *Server) deleteSecret(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, sec, ok := s.secret(w, r); ok {
		delete(st.secrets, sec.id)
		rtfake.JSON(w, http.StatusOK, "", envelope(sec.json(st.id), nil))
	}
}

func (s *Server) duplicateSecret(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string   `json:"name"`
		Scopes  []string `json:"scopes"`
		Comment string   `json:"comment"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.mu.Lock()
	defer s.mu.Unlock()
	st, src, ok := s.secret(w, r)
	if !ok {
		return
	}
	if st.byName(in.Name) != nil {
		fail(w, http.StatusConflict, 10000, fmt.Sprintf("secret %q already exists", in.Name))
		return
	}
	now := s.now()
	dup := &secret{id: s.newID(), name: in.Name, value: src.value, scopes: in.Scopes, comment: in.Comment, created: now, modified: now}
	st.secrets[dup.id] = dup
	rtfake.JSON(w, http.StatusOK, "", envelope(dup.json(st.id), nil))
}
