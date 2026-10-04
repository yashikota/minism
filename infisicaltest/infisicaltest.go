// Package infisicaltest gives tests an in-memory implementation of the Infisical
// Go SDK's own client interfaces (infisical.InfisicalClientInterface and
// friends). The SDK's concrete client keeps its API fields private, so the
// public interfaces are the supported seam.
//
// Secrets and Folders are implemented; Auth is a permissive fake. DynamicSecrets,
// Kms and Ssh are not implemented and panic if used.
package infisicaltest

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	infisical "github.com/infisical/go-sdk"
	"github.com/infisical/go-sdk/packages/models"

	"github.com/yashikota/minism/internal/memstore"
)

// Server is an in-memory Infisical backend.
type Server struct {
	mu      sync.Mutex
	secrets *memstore.Store[models.Secret]
	folders *memstore.Store[models.Folder]
	seq     int
}

// New returns an empty backend.
func New(t testing.TB) *Server {
	t.Helper()
	return &Server{secrets: memstore.New[models.Secret](), folders: memstore.New[models.Folder]()}
}

// Client returns the in-memory client.
func (s *Server) Client() infisical.InfisicalClientInterface {
	return &client{s: s, auth: &auth{}}
}

// Value reads a secret directly from the store (fixture inspection).
func (s *Server) Value(project, env, secretPath, key string) (string, bool) {
	v, err := s.secrets.Latest(skey(project, env, secretPath, key))
	return v.Value.SecretValue, err == nil
}

func normPath(p string) string {
	if p == "" {
		return "/"
	}
	return path.Clean("/" + strings.Trim(p, "/"))
}

func project(id, slug string) string {
	if id != "" {
		return id
	}
	return slug
}

func skey(project, env, p, name string) string {
	return strings.Join([]string{project, env, normPath(p), name}, "\x00")
}

func apiErr(op string, status int, msg string) error {
	return &infisical.APIError{Operation: op, Method: "GET", URL: "https://infisical.invalid", StatusCode: status, ErrorMessage: msg, ReqId: "minism"}
}

func notFound(op, what string) error { return apiErr(op, http.StatusNotFound, what+" not found") }

type client struct {
	s    *Server
	auth *auth
}

func (c *client) UpdateConfiguration(infisical.Config)              {}
func (c *client) Secrets() infisical.SecretsInterface               { return &secretsAPI{c.s} }
func (c *client) Folders() infisical.FoldersInterface               { return &foldersAPI{c.s} }
func (c *client) Auth() infisical.AuthInterface                     { return c.auth }
func (c *client) DynamicSecrets() infisical.DynamicSecretsInterface { return unimplementedDynamic{} }
func (c *client) Kms() infisical.KmsInterface                       { return unimplementedKms{} }
func (c *client) Ssh() infisical.SshInterface                       { return unimplementedSsh{} }

type unimplementedDynamic struct {
	infisical.DynamicSecretsInterface
}
type unimplementedKms struct{ infisical.KmsInterface }
type unimplementedSsh struct{ infisical.SshInterface }

// ---- secrets ----

type secretsAPI struct{ s *Server }

func (a *secretsAPI) fill(sec models.Secret, project, env, p string, ver int) models.Secret {
	sec.Workspace, sec.Environment, sec.SecretPath, sec.Version = project, env, normPath(p), ver
	return sec
}

func (a *secretsAPI) Create(o infisical.CreateSecretOptions) (models.Secret, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	a.s.seq++
	typ := o.Type
	if typ == "" {
		typ = "shared"
	}
	sec := models.Secret{
		ID: fmt.Sprintf("secret-%d", a.s.seq), SecretKey: o.SecretKey, SecretValue: o.SecretValue,
		SecretComment: o.SecretComment, Type: typ,
	}
	v, err := a.s.secrets.Create(skey(o.ProjectID, o.Environment, o.SecretPath, o.SecretKey), sec)
	if errors.Is(err, memstore.ErrExists) {
		return models.Secret{}, apiErr("CallCreateSecretsV3", http.StatusBadRequest, "Secret already exists")
	}
	return a.fill(v.Value, o.ProjectID, o.Environment, o.SecretPath, v.Num), nil
}

func (a *secretsAPI) Retrieve(o infisical.RetrieveSecretOptions) (models.Secret, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	proj := project(o.ProjectID, o.ProjectSlug)
	key := skey(proj, o.Environment, o.SecretPath, o.SecretKey)
	var (
		v   memstore.Version[models.Secret]
		err error
	)
	if o.Version > 0 {
		v, err = a.s.secrets.At(key, o.Version)
	} else {
		v, err = a.s.secrets.Latest(key)
	}
	if err != nil {
		return models.Secret{}, notFound("CallRetrieveSecretV3", "Secret")
	}
	return a.fill(v.Value, proj, o.Environment, o.SecretPath, v.Num), nil
}

func (a *secretsAPI) Update(o infisical.UpdateSecretOptions) (models.Secret, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	key := skey(o.ProjectID, o.Environment, o.SecretPath, o.SecretKey)
	cur, err := a.s.secrets.Latest(key)
	if err != nil {
		return models.Secret{}, notFound("CallUpdateSecretV3", "Secret")
	}
	next := cur.Value
	if o.NewSecretValue != "" {
		next.SecretValue = o.NewSecretValue
	}
	v, _ := a.s.secrets.Append(key, next)
	return a.fill(v.Value, o.ProjectID, o.Environment, o.SecretPath, v.Num), nil
}

func (a *secretsAPI) Delete(o infisical.DeleteSecretOptions) (models.Secret, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	key := skey(o.ProjectID, o.Environment, o.SecretPath, o.SecretKey)
	cur, err := a.s.secrets.Latest(key)
	if err != nil || a.s.secrets.Delete(key) != nil {
		return models.Secret{}, notFound("CallDeleteSecretV3", "Secret")
	}
	return a.fill(cur.Value, o.ProjectID, o.Environment, o.SecretPath, cur.Num), nil
}

func (a *secretsAPI) ListSecrets(o infisical.ListSecretsOptions) (infisical.ListSecretsResult, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	proj := project(o.ProjectID, o.ProjectSlug)
	base := normPath(o.SecretPath)
	var out []models.Secret
	for _, k := range a.s.secrets.Keys() { // sorted
		parts := strings.Split(k, "\x00")
		if parts[0] != proj || parts[1] != o.Environment {
			continue
		}
		dir := parts[2]
		in := dir == base
		if o.Recursive {
			in = dir == base || strings.HasPrefix(dir, strings.TrimSuffix(base, "/")+"/")
		}
		if !in {
			continue
		}
		v, _ := a.s.secrets.Latest(k)
		out = append(out, a.fill(v.Value, proj, o.Environment, dir, v.Num))
	}
	h := sha256.New()
	for _, s := range out {
		fmt.Fprintf(h, "%s\x00%s\x00%d\n", s.SecretPath, s.SecretKey, s.Version)
	}
	return infisical.ListSecretsResult{Secrets: out, ETag: fmt.Sprintf("%x", h.Sum(nil)[:8])}, nil
}

func (a *secretsAPI) List(o infisical.ListSecretsOptions) ([]models.Secret, error) {
	r, err := a.ListSecrets(o)
	return r.Secrets, err
}

func (a *secretsAPI) Batch() infisical.BatchSecretsInterface { return &batch{a} }

type batch struct{ a *secretsAPI }

func (b *batch) Create(o infisical.BatchCreateSecretsOptions) ([]models.Secret, error) {
	var out []models.Secret
	for _, s := range o.Secrets {
		sec, err := b.a.Create(infisical.CreateSecretOptions{
			ProjectID: o.ProjectID, Environment: o.Environment, SecretPath: o.SecretPath,
			SecretKey: s.SecretKey, SecretValue: s.SecretValue, SecretComment: s.SecretComment,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, sec)
	}
	return out, nil
}

// ---- folders ----

type foldersAPI struct{ s *Server }

func fkey(project, env, p, name string) string { return skey(project, env, p, name) }

func (a *foldersAPI) Create(o infisical.CreateFolderOptions) (models.Folder, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	a.s.seq++
	f := models.Folder{ID: fmt.Sprintf("folder-%d", a.s.seq), Name: o.Name, Version: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), EnvironmentID: o.Environment}
	if _, err := a.s.folders.Create(fkey(o.ProjectID, o.Environment, o.Path, o.Name), f); err != nil {
		return models.Folder{}, apiErr("CallCreateFolder", http.StatusBadRequest, "Folder already exists")
	}
	return f, nil
}

func (a *foldersAPI) List(o infisical.ListFoldersOptions) ([]models.Folder, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	var out []models.Folder
	for _, k := range a.s.folders.Keys() {
		parts := strings.Split(k, "\x00")
		if parts[0] == o.ProjectID && parts[1] == o.Environment && parts[2] == normPath(o.Path) {
			v, _ := a.s.folders.Latest(k)
			out = append(out, v.Value)
		}
	}
	return out, nil
}

func (a *foldersAPI) find(project, env, p, id, name string) (string, models.Folder, bool) {
	for _, k := range a.s.folders.Keys() {
		parts := strings.Split(k, "\x00")
		if parts[0] != project || parts[1] != env || parts[2] != normPath(p) {
			continue
		}
		v, _ := a.s.folders.Latest(k)
		if (id != "" && v.Value.ID == id) || (id == "" && v.Value.Name == name) {
			return k, v.Value, true
		}
	}
	return "", models.Folder{}, false
}

func (a *foldersAPI) Update(o infisical.UpdateFolderOptions) (models.Folder, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	k, f, ok := a.find(o.ProjectID, o.Environment, o.Path, o.FolderID, "")
	if !ok {
		return models.Folder{}, notFound("CallUpdateFolder", "Folder")
	}
	_ = a.s.folders.Delete(k)
	f.Name, f.Version, f.UpdatedAt = o.NewName, f.Version+1, time.Now().UTC()
	_, _ = a.s.folders.Create(fkey(o.ProjectID, o.Environment, o.Path, o.NewName), f)
	return f, nil
}

func (a *foldersAPI) Delete(o infisical.DeleteFolderOptions) (models.Folder, error) {
	a.s.mu.Lock()
	defer a.s.mu.Unlock()
	k, f, ok := a.find(o.ProjectID, o.Environment, o.Path, o.FolderID, o.FolderName)
	if !ok {
		return models.Folder{}, notFound("CallDeleteFolder", "Folder")
	}
	_ = a.s.folders.Delete(k)
	return f, nil
}

// ---- auth ----

type auth struct {
	mu    sync.Mutex
	token string
	org   string
}

func (a *auth) login() (infisical.MachineIdentityCredential, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.token = "minism-access-token"
	return infisical.MachineIdentityCredential{AccessToken: a.token, ExpiresIn: 7200, AccessTokenMaxTTL: 7200, TokenType: "Bearer"}, nil
}

func (a *auth) SetAccessToken(t string) { a.mu.Lock(); a.token = t; a.mu.Unlock() }
func (a *auth) GetAccessToken() string  { a.mu.Lock(); defer a.mu.Unlock(); return a.token }
func (a *auth) GetOrganizationSlug() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.org
}

func (a *auth) WithOrganizationSlug(s string) infisical.AuthInterface {
	a.mu.Lock()
	a.org = s
	a.mu.Unlock()
	return a
}
func (a *auth) WithAzureClientID(string) infisical.AuthInterface { return a }

func (a *auth) UniversalAuthLogin(string, string) (infisical.MachineIdentityCredential, error) {
	return a.login()
}
func (a *auth) JwtAuthLogin(string, string) (infisical.MachineIdentityCredential, error) {
	return a.login()
}
func (a *auth) KubernetesAuthLogin(string, string) (infisical.MachineIdentityCredential, error) {
	return a.login()
}
func (a *auth) KubernetesRawServiceAccountTokenLogin(string, string) (infisical.MachineIdentityCredential, error) {
	return a.login()
}
func (a *auth) AzureAuthLogin(string, string) (infisical.MachineIdentityCredential, error) {
	return a.login()
}
func (a *auth) GcpIdTokenAuthLogin(string) (infisical.MachineIdentityCredential, error) {
	return a.login()
}
func (a *auth) GcpIamAuthLogin(string, string) (infisical.MachineIdentityCredential, error) {
	return a.login()
}
func (a *auth) AwsIamAuthLogin(string) (infisical.MachineIdentityCredential, error) { return a.login() }
func (a *auth) OidcAuthLogin(string, string) (infisical.MachineIdentityCredential, error) {
	return a.login()
}
func (a *auth) OciAuthLogin(infisical.OciAuthLoginOptions) (infisical.MachineIdentityCredential, error) {
	return a.login()
}
func (a *auth) LdapAuthLogin(string, string, string) (infisical.MachineIdentityCredential, error) {
	return a.login()
}
func (a *auth) RevokeAccessToken() error { a.SetAccessToken(""); return nil }
