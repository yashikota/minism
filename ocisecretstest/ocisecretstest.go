// Package ocisecretstest gives tests real OCI SDK clients whose HTTP dispatcher
// is an in-memory OCI Vault.
//
// OCI splits secrets over two official clients: vault.VaultsClient manages
// secrets, secrets.SecretsClient reads secret bundles. Env returns both, sharing
// one state. OCI publishes no server implementation, so the state is ours: it
// covers create/get/update/list/versions, scheduled deletion and bundle reads
// (by id and by name, by stage or version number). Other operations answer 501.
package ocisecretstest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/secrets"
	"github.com/oracle/oci-go-sdk/v65/vault"

	"github.com/yashikota/minism/internal/rtfake"
)

const region = "us-ashburn-1"

type version struct {
	num     int64
	name    string
	content string // base64, as OCI stores and returns it
	created time.Time
	current bool
	prev    bool
}

type secret struct {
	id, compartment, name, vaultID, keyID, desc string
	free                                        map[string]string
	versions                                    []*version
	created                                     time.Time
	deleting                                    *time.Time
}

// Server is an in-memory OCI Vault service.
type Server struct {
	t       testing.TB
	mu      sync.Mutex
	secrets map[string]*secret
	seq     int
	now     func() time.Time
	mux     *http.ServeMux
}

// Env holds the two official clients sharing one Server's state.
type Env struct {
	Vault   *vault.VaultsClient
	Secrets *secrets.SecretsClient
	// IDs to put in requests; the Server does not validate them.
	CompartmentID, VaultID, KeyID string
}

var (
	keyOnce sync.Once
	keyPEM  string
)

func privateKey() string {
	keyOnce.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}))
	})
	return keyPEM
}

// New returns an empty service.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{t: t, secrets: map[string]*secret{}, now: func() time.Time { return time.Now().UTC() }}
	m := http.NewServeMux()
	m.HandleFunc("POST /20180608/secrets", s.create)
	m.HandleFunc("GET /20180608/secrets", s.list)
	m.HandleFunc("GET /20180608/secrets/{id}", s.get)
	m.HandleFunc("PUT /20180608/secrets/{id}", s.update)
	m.HandleFunc("POST /20180608/secrets/{id}/actions/scheduleDeletion", s.scheduleDeletion)
	m.HandleFunc("POST /20180608/secrets/{id}/actions/cancelDeletion", s.cancelDeletion)
	m.HandleFunc("GET /20180608/secrets/{id}/versions", s.listVersions)
	m.HandleFunc("GET /20180608/secrets/{id}/version/{num}", s.getVersion)
	m.HandleFunc("GET /20190301/secretbundles/{id}", s.bundle)
	m.HandleFunc("GET /20190301/secretbundles/actions/getByName", s.bundleByName)
	m.HandleFunc("POST /20190301/secretbundles/actions/getByName", s.bundleByName)
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, http.StatusNotImplemented, "NotImplemented", fmt.Sprintf("minism: oci %s %s not implemented", r.Method, r.URL.Path))
	})
	s.mux = m
	return s
}

type dispatcher struct{ rt http.RoundTripper }

func (d dispatcher) Do(req *http.Request) (*http.Response, error) { return d.rt.RoundTrip(req) }

// Env returns the official clients wired to this Server.
func (s *Server) Env() *Env {
	s.t.Helper()
	provider := common.NewRawConfigurationProvider("ocid1.tenancy.oc1..minism", "ocid1.user.oc1..minism", region,
		"00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff", privateKey(), nil)
	d := dispatcher{rtfake.Transport(s.mux)}

	vc, err := vault.NewVaultsClientWithConfigurationProvider(provider)
	if err != nil {
		s.t.Fatalf("ocisecretstest: vaults client: %v", err)
	}
	vc.HTTPClient = d
	sc, err := secrets.NewSecretsClientWithConfigurationProvider(provider)
	if err != nil {
		s.t.Fatalf("ocisecretstest: secrets client: %v", err)
	}
	sc.HTTPClient = d
	return &Env{
		Vault: &vc, Secrets: &sc,
		CompartmentID: "ocid1.compartment.oc1..minism",
		VaultID:       "ocid1.vault.oc1.iad.minism",
		KeyID:         "ocid1.key.oc1.iad.minism",
	}
}

// Content returns the base64 content of the CURRENT version (fixture inspection).
func (s *Server) Content(name string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sec := range s.secrets {
		if sec.name == name {
			if v := sec.current(); v != nil {
				return v.content, true
			}
		}
	}
	return "", false
}

func fail(w http.ResponseWriter, status int, code, msg string) {
	rtfake.JSON(w, status, "", map[string]string{"code": code, "message": msg})
}

func notFound(w http.ResponseWriter) {
	fail(w, http.StatusNotFound, "NotAuthorizedOrNotFound", "Authorization failed or requested resource not found.")
}

func ts(t time.Time) string { return t.Format("2006-01-02T15:04:05.000Z") }

func (sec *secret) current() *version {
	for _, v := range sec.versions {
		if v.current {
			return v
		}
	}
	return nil
}

func (sec *secret) state() string {
	if sec.deleting != nil {
		return "PENDING_DELETION"
	}
	return "ACTIVE"
}

func (sec *secret) json(summary bool) map[string]any {
	m := map[string]any{
		"id": sec.id, "compartmentId": sec.compartment, "secretName": sec.name, "vaultId": sec.vaultID,
		"keyId": sec.keyID, "description": sec.desc, "lifecycleState": sec.state(), "timeCreated": ts(sec.created),
		"freeformTags": sec.free, "definedTags": map[string]any{},
	}
	if sec.free == nil {
		m["freeformTags"] = map[string]string{}
	}
	if sec.deleting != nil {
		m["timeOfDeletion"] = ts(*sec.deleting)
	}
	if !summary {
		m["currentVersionNumber"] = sec.current().num
	}
	return m
}

func (v *version) stages() []string {
	var out []string
	if v.current {
		out = append(out, "CURRENT")
	}
	if v.prev {
		out = append(out, "PREVIOUS")
	}
	return out
}

func (sec *secret) addVersion(content, name string, now time.Time) *version {
	for _, v := range sec.versions {
		v.prev = v.current // the old CURRENT becomes PREVIOUS; older ones lose the label
		v.current = false
	}
	v := &version{num: int64(len(sec.versions) + 1), name: name, content: content, created: now, current: true}
	sec.versions = append(sec.versions, v)
	return v
}

type contentIn struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func (s *Server) newID() string {
	s.seq++
	return fmt.Sprintf("ocid1.vaultsecret.oc1.iad.minism%06d", s.seq)
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CompartmentID string            `json:"compartmentId"`
		SecretName    string            `json:"secretName"`
		VaultID       string            `json:"vaultId"`
		KeyID         string            `json:"keyId"`
		Description   string            `json:"description"`
		FreeformTags  map[string]string `json:"freeformTags"`
		SecretContent contentIn         `json:"secretContent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "InvalidParameter", err.Error())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sec := range s.secrets {
		if sec.name == in.SecretName && sec.vaultID == in.VaultID {
			fail(w, http.StatusConflict, "Conflict", "Secret with the same name already exists in the vault.")
			return
		}
	}
	now := s.now()
	sec := &secret{
		id: s.newID(), compartment: in.CompartmentID, name: in.SecretName, vaultID: in.VaultID, keyID: in.KeyID,
		desc: in.Description, free: in.FreeformTags, created: now,
	}
	sec.addVersion(in.SecretContent.Content, in.SecretContent.Name, now)
	s.secrets[sec.id] = sec
	rtfake.JSON(w, http.StatusOK, "", sec.json(false))
}

func (s *Server) find(w http.ResponseWriter, r *http.Request) (*secret, bool) {
	sec, ok := s.secrets[r.PathValue("id")]
	if !ok {
		notFound(w)
	}
	return sec, ok
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sec, ok := s.find(w, r); ok {
		rtfake.JSON(w, http.StatusOK, "", sec.json(false))
	}
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Description   *string           `json:"description"`
		FreeformTags  map[string]string `json:"freeformTags"`
		SecretContent *contentIn        `json:"secretContent"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.find(w, r)
	if !ok {
		return
	}
	if in.Description != nil {
		sec.desc = *in.Description
	}
	if in.FreeformTags != nil {
		sec.free = in.FreeformTags
	}
	if in.SecretContent != nil {
		sec.addVersion(in.SecretContent.Content, in.SecretContent.Name, s.now())
	}
	rtfake.JSON(w, http.StatusOK, "", sec.json(false))
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []*secret
	for _, sec := range s.secrets {
		if c := q.Get("compartmentId"); c != "" && sec.compartment != c {
			continue
		}
		if n := q.Get("name"); n != "" && sec.name != n {
			continue
		}
		if v := q.Get("vaultId"); v != "" && sec.vaultID != v {
			continue
		}
		if st := q.Get("lifecycleState"); st != "" && sec.state() != st {
			continue
		}
		all = append(all, sec)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].name < all[j].name })
	out := []map[string]any{}
	for _, sec := range all {
		out = append(out, sec.json(true))
	}
	rtfake.JSON(w, http.StatusOK, "", out)
}

func (s *Server) scheduleDeletion(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TimeOfDeletion *string `json:"timeOfDeletion"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.find(w, r)
	if !ok {
		return
	}
	when := s.now().AddDate(0, 0, 30)
	if in.TimeOfDeletion != nil {
		if t, err := time.Parse(time.RFC3339Nano, *in.TimeOfDeletion); err == nil {
			when = t
		}
	}
	sec.deleting = &when
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) cancelDeletion(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sec, ok := s.find(w, r); ok {
		sec.deleting = nil
		w.WriteHeader(http.StatusAccepted)
	}
}

func (v *version) summary(id string) map[string]any {
	return map[string]any{
		"secretId": id, "versionNumber": v.num, "timeCreated": ts(v.created),
		"stages": v.stages(), "name": v.name,
	}
}

func (s *Server) listVersions(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.find(w, r)
	if !ok {
		return
	}
	out := []map[string]any{}
	for _, v := range sec.versions {
		out = append(out, v.summary(sec.id))
	}
	rtfake.JSON(w, http.StatusOK, "", out)
}

func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.find(w, r)
	if !ok {
		return
	}
	n, _ := strconv.Atoi(r.PathValue("num"))
	if n < 1 || n > len(sec.versions) {
		notFound(w)
		return
	}
	rtfake.JSON(w, http.StatusOK, "", sec.versions[n-1].summary(sec.id))
}

// pick selects a version by number, stage (default CURRENT) or version name.
func (sec *secret) pick(q map[string][]string) *version {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	if n, err := strconv.Atoi(get("versionNumber")); err == nil {
		if n >= 1 && n <= len(sec.versions) {
			return sec.versions[n-1]
		}
		return nil
	}
	if name := get("secretVersionName"); name != "" {
		for _, v := range sec.versions {
			if v.name == name {
				return v
			}
		}
		return nil
	}
	switch get("stage") {
	case "", "CURRENT":
		return sec.current()
	case "LATEST":
		return sec.versions[len(sec.versions)-1]
	case "PREVIOUS":
		for _, v := range sec.versions {
			if v.prev {
				return v
			}
		}
	}
	return nil
}

func (sec *secret) bundleJSON(v *version) map[string]any {
	m := map[string]any{
		"secretId": sec.id, "versionNumber": v.num, "timeCreated": ts(v.created), "stages": v.stages(),
		"secretBundleContent": map[string]string{"contentType": "BASE64", "content": v.content},
	}
	if v.name != "" {
		m["versionName"] = v.name
	}
	return m
}

func (s *Server) bundle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sec, ok := s.find(w, r)
	if !ok {
		return
	}
	if v := sec.pick(r.URL.Query()); v != nil && sec.deleting == nil {
		rtfake.JSON(w, http.StatusOK, "", sec.bundleJSON(v))
		return
	}
	notFound(w)
}

func (s *Server) bundleByName(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sec := range s.secrets {
		if sec.name == q.Get("secretName") && sec.vaultID == q.Get("vaultId") && sec.deleting == nil {
			if v := sec.pick(q); v != nil {
				rtfake.JSON(w, http.StatusOK, "", sec.bundleJSON(v))
				return
			}
		}
	}
	notFound(w)
}
