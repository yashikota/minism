// Package keepertest gives tests a real *core.SecretsManager (Keeper Secrets
// Manager Go SDK) talking to an in-memory Keeper endpoint.
//
// The SDK has a test-only seam: NewSecretsManager accepts a **core.Context whose
// Transport replaces the network, and PostQuery writes each request's raw
// transmission key into that shared Context. The server reads the key from there
// (Keeper's own public key never has to be broken), decrypts the request, and
// encrypts the answer, all with the SDK's own crypto helpers.
//
// State is ours: one shared folder holding records, each encrypted with its own
// record key. Supported: get_secret, get_folders, create_secret, update_secret,
// delete_secret, finalize/rollback (no-ops). Other routes answer 501.
package keepertest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/keeper-security/secrets-manager-go/core"

	"github.com/yashikota/minism/internal/rtfake"
)

const hostname = "keeper.invalid"

type record struct {
	uid      string
	key      []byte
	json     string
	revision int64
}

// Server is an in-memory Keeper Secrets Manager application with one shared folder.
type Server struct {
	t         testing.TB
	mu        sync.Mutex
	appKey    []byte
	folderUID string
	folderKey []byte
	records   map[string]*record
	config    string // JSON client configuration
}

// New returns a Server with an empty shared folder.
func New(t testing.TB) *Server {
	t.Helper()
	appKey, err := core.GetRandomBytes(32)
	if err != nil {
		t.Fatal(err)
	}
	folderKey, err := core.GetRandomBytes(32)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{t: t, appKey: appKey, folderKey: folderKey, folderUID: core.GenerateUid(), records: map[string]*record{}}

	// Let the SDK generate a valid client identity (client id, ECC key pair), then
	// pre-bind it: install the app key so no binding round trip is needed.
	cfg := core.NewMemoryKeyValueStorage()
	core.NewSecretsManager(&core.ClientOptions{
		Token: "US:" + core.BytesToUrlSafeStr(mustRandom(t, 32)), Hostname: hostname, InsecureSkipVerify: true, Config: cfg,
	})
	cfg.Set(core.KEY_APP_KEY, core.BytesToBase64(appKey))
	cfg.Set(core.KEY_OWNER_PUBLIC_KEY, core.GetDefaultOwnerPublicKey())
	cfg.Delete(core.KEY_CLIENT_KEY)
	out := map[string]string{}
	for _, k := range core.GetConfigKeys() {
		if cfg.Contains(k) {
			out[string(k)] = cfg.Get(k)
		}
	}
	b, _ := json.Marshal(out)
	s.config = string(b)
	return s
}

func mustRandom(t testing.TB, n int) []byte {
	b, err := core.GetRandomBytes(n)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// FolderUID is the UID of the shared folder new records are created in.
func (s *Server) FolderUID() string { return s.folderUID }

// Client returns a real SDK client bound to this Server.
func (s *Server) Client() *core.SecretsManager {
	s.t.Helper()
	ctx := &core.Context{}
	ctxp := &ctx
	ctx.Transport = rtfake.Transport(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.serve(w, r, (*ctxp).TransmissionKey.Key)
	}))
	sm := core.NewSecretsManager(&core.ClientOptions{Config: core.NewMemoryKeyValueStorage(s.config), InsecureSkipVerify: true}, ctxp)
	if sm == nil {
		s.t.Fatal("keepertest: SDK refused the generated configuration")
	}
	return sm
}

// Seed adds a record to the shared folder and returns its UID (fixture helper).
func (s *Server) Seed(rc *core.RecordCreate) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := json.Marshal(rc)
	if err != nil {
		s.t.Fatal(err)
	}
	uid := core.GenerateUid()
	s.records[uid] = &record{uid: uid, key: mustRandom(s.t, 32), json: string(raw), revision: 1}
	return uid
}

// RecordJSON returns the decrypted record data (fixture inspection).
func (s *Server) RecordJSON(uid string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[uid]
	if !ok {
		return "", false
	}
	return r.json, true
}

// Len counts records.
func (s *Server) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.records)
}

func (s *Server) reply(w http.ResponseWriter, key []byte, v any) {
	plain, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	enc, err := core.EncryptAesGcm(plain, key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(enc)
}

func fail(w http.ResponseWriter, status int, code, msg string) {
	rtfake.JSON(w, status, "", map[string]string{"error": code, "message": msg})
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request, key []byte) {
	route := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	body, _ := io.ReadAll(r.Body)
	plain, err := core.Decrypt(body, key)
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "cannot decrypt payload: "+err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch route {
	case "get_secret":
		s.getSecret(w, key, plain)
	case "get_folders":
		s.getFolders(w, key)
	case "create_secret":
		s.createSecret(w, key, plain)
	case "update_secret":
		s.updateSecret(w, key, plain)
	case "delete_secret":
		s.deleteSecret(w, key, plain)
	case "finalize_secret_update", "rollback_secret_update":
		w.WriteHeader(http.StatusOK)
	default:
		fail(w, http.StatusNotImplemented, "not_implemented", fmt.Sprintf("minism: keeper %s not implemented", route))
	}
}

func (s *Server) recordJSON(r *record) map[string]any {
	data, _ := core.EncryptAesGcm([]byte(r.json), r.key)
	rk, _ := core.EncryptAesGcm(r.key, s.folderKey)
	return map[string]any{
		"recordUid": r.uid, "recordKey": core.BytesToBase64(rk), "data": core.BytesToBase64(data),
		"revision": r.revision, "isEditable": true, "files": []any{},
	}
}

func (s *Server) getSecret(w http.ResponseWriter, key, plain []byte) {
	var in struct {
		RequestedRecords []string `json:"requestedRecords"`
	}
	_ = json.Unmarshal(plain, &in)
	want := map[string]bool{}
	for _, u := range in.RequestedRecords {
		want[u] = true
	}
	uids := make([]string, 0, len(s.records))
	for u := range s.records {
		if len(want) == 0 || want[u] {
			uids = append(uids, u)
		}
	}
	sort.Strings(uids)
	recs := []any{}
	for _, u := range uids {
		recs = append(recs, s.recordJSON(s.records[u]))
	}
	fk, _ := core.EncryptAesGcm(s.folderKey, s.appKey)
	s.reply(w, key, map[string]any{
		"records": []any{},
		"folders": []any{map[string]any{"folderUid": s.folderUID, "folderKey": core.BytesToBase64(fk), "records": recs}},
	})
}

func (s *Server) getFolders(w http.ResponseWriter, key []byte) {
	fk, _ := core.EncryptAesGcm(s.folderKey, s.appKey)
	name, _ := core.EncryptAesCbc([]byte(`{"name":"folder"}`), s.folderKey)
	s.reply(w, key, map[string]any{"folders": []any{map[string]any{
		"folderUid": s.folderUID, "folderKey": core.BytesToUrlSafeStr(fk), "data": core.BytesToUrlSafeStr(name),
	}}})
}

func (s *Server) createSecret(w http.ResponseWriter, key, plain []byte) {
	var in struct {
		RecordUid, FolderUid, FolderKey, Data string
	}
	var raw map[string]string
	if err := json.Unmarshal(plain, &raw); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	in.RecordUid, in.FolderUid, in.FolderKey, in.Data = raw["recordUid"], raw["folderUid"], raw["folderKey"], raw["data"]
	if in.FolderUid != s.folderUID {
		fail(w, http.StatusBadRequest, "bad_request", "unknown folder "+in.FolderUid)
		return
	}
	// The record key reaches us wrapped with the folder key; the copy wrapped for
	// the app owner's public key is unreadable here, and not needed.
	recKey, err := core.Decrypt(core.Base64ToBytes(in.FolderKey), s.folderKey)
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "cannot unwrap record key: "+err.Error())
		return
	}
	data, err := core.Decrypt(core.Base64ToBytes(in.Data), recKey)
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "cannot decrypt record data: "+err.Error())
		return
	}
	if _, dup := s.records[in.RecordUid]; dup {
		fail(w, http.StatusBadRequest, "record_exists", "record already exists")
		return
	}
	s.records[in.RecordUid] = &record{uid: in.RecordUid, key: recKey, json: string(bytes.TrimSpace(data)), revision: 1}
	s.reply(w, key, map[string]any{})
}

func (s *Server) updateSecret(w http.ResponseWriter, key, plain []byte) {
	var in struct {
		RecordUid string `json:"recordUid"`
		Revision  int64  `json:"revision"`
		Data      string `json:"data"`
	}
	if err := json.Unmarshal(plain, &in); err != nil {
		fail(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	r, ok := s.records[in.RecordUid]
	if !ok {
		fail(w, http.StatusBadRequest, "access_denied", "record not found")
		return
	}
	if in.Revision != r.revision {
		fail(w, http.StatusBadRequest, "invalid_revision", fmt.Sprintf("record revision %d is stale (current %d)", in.Revision, r.revision))
		return
	}
	data, err := core.Decrypt(core.UrlSafeStrToBytes(in.Data), r.key)
	if err != nil {
		fail(w, http.StatusBadRequest, "bad_request", "cannot decrypt record data: "+err.Error())
		return
	}
	r.json, r.revision = string(data), r.revision+1
	s.reply(w, key, map[string]any{})
}

func (s *Server) deleteSecret(w http.ResponseWriter, key, plain []byte) {
	var in struct {
		RecordUids []string `json:"recordUids"`
	}
	_ = json.Unmarshal(plain, &in)
	out := []map[string]string{}
	for _, uid := range in.RecordUids {
		if _, ok := s.records[uid]; ok {
			delete(s.records, uid)
			out = append(out, map[string]string{"recordUid": uid, "responseCode": "ok", "errorMessage": ""})
		} else {
			out = append(out, map[string]string{"recordUid": uid, "responseCode": "access_denied", "errorMessage": "Record does not exist"})
		}
	}
	s.reply(w, key, map[string]any{"records": out})
}
