// Package azsecretstest gives tests a real *azsecrets.Client backed by
// Microsoft's own azsecrets/fake server transport and an in-memory store.
//
// The only code here is the store: Microsoft's fake.ServerTransport already
// implements the Key Vault REST protocol, so no HTTP is written by hand.
package azsecretstest

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets/fake"

	"github.com/yashikota/minism/internal/memstore"
)

const vaultURL = "https://minism.vault.azure.net"

type secret struct {
	value       string
	contentType *string
	tags        map[string]*string
	enabled     *bool
	created     time.Time
	updated     time.Time
}

// Server is an in-memory Key Vault secrets store.
type Server struct {
	t     testing.TB
	store *memstore.Store[secret]
	srv   fake.Server
}

// New returns a Server ready to hand out clients.
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{t: t, store: memstore.New[secret]()}
	s.srv = fake.Server{
		SetSecret:                            s.setSecret,
		GetSecret:                            s.getSecret,
		DeleteSecret:                         s.deleteSecret,
		UpdateSecretProperties:               s.updateSecretProperties,
		NewListSecretPropertiesPager:         s.listSecrets,
		NewListSecretPropertiesVersionsPager: s.listVersions,
	}
	return s
}

// Client returns a real azsecrets.Client wired to the in-memory server.
func (s *Server) Client() *azsecrets.Client {
	s.t.Helper()
	c, err := azsecrets.NewClient(vaultURL, &azfake.TokenCredential{}, &azsecrets.ClientOptions{
		ClientOptions: azcore.ClientOptions{Transport: fake.NewServerTransport(&s.srv)},
	})
	if err != nil {
		s.t.Fatalf("azsecretstest: new client: %v", err)
	}
	return c
}

// Value reads the latest value directly from the store (fixture inspection).
func (s *Server) Value(name string) (string, bool) {
	v, err := s.store.Latest(name)
	return v.Value.value, err == nil
}

// Len counts live secrets.
func (s *Server) Len() int { return s.store.Len() }

// Azure version ids are 32 hex chars; encode the store's version number in one.
func versionID(n int) string { return fmt.Sprintf("%032x", n) }

func versionNum(id string) (int, bool) {
	n, err := strconv.ParseInt(id, 16, 32)
	return int(n), err == nil
}

func id(name string, n int) *azsecrets.ID {
	return to.Ptr(azsecrets.ID(fmt.Sprintf("%s/secrets/%s/%s", vaultURL, name, versionID(n))))
}

func attrs(v secret) *azsecrets.SecretAttributes {
	return &azsecrets.SecretAttributes{Enabled: v.enabled, Created: &v.created, Updated: &v.updated}
}

func bundle(name string, v memstore.Version[secret]) azsecrets.Secret {
	return azsecrets.Secret{
		ID: id(name, v.Num), Value: &v.Value.value, ContentType: v.Value.contentType,
		Tags: v.Value.tags, Attributes: attrs(v.Value),
	}
}

func props(name string, v memstore.Version[secret]) *azsecrets.SecretProperties {
	return &azsecrets.SecretProperties{
		ID: id(name, v.Num), ContentType: v.Value.contentType, Tags: v.Value.tags, Attributes: attrs(v.Value),
	}
}

func notFound(r *azfake.ErrorResponder) { r.SetResponseError(http.StatusNotFound, "SecretNotFound") }

// splitName works around a bug in azsecrets/fake v1.5.0: its path regexp has a
// character class containing the range `$-;`, which includes '/', so the name
// group swallows the version ("plain/0001" arrives as name, with version "").
// Key Vault secret names never contain '/', so splitting at the first one is
// exact, and it is a no-op once upstream is fixed.
func splitName(name, version string) (string, string) {
	if i := strings.IndexByte(name, '/'); i >= 0 {
		return name[:i], name[i+1:]
	}
	return name, version
}

func (s *Server) setSecret(_ context.Context, name string, p azsecrets.SetSecretParameters, _ *azsecrets.SetSecretOptions) (resp azfake.Responder[azsecrets.SetSecretResponse], errResp azfake.ErrorResponder) {
	now := time.Now().UTC()
	v := secret{contentType: p.ContentType, tags: p.Tags, created: now, updated: now, enabled: to.Ptr(true)}
	if p.Value != nil {
		v.value = *p.Value
	}
	if p.SecretAttributes != nil && p.SecretAttributes.Enabled != nil {
		v.enabled = p.SecretAttributes.Enabled
	}
	ver := s.store.Put(name, v)
	resp.SetResponse(http.StatusOK, azsecrets.SetSecretResponse{Secret: bundle(name, memstore.Version[secret]{Num: ver.Num, Value: v})}, nil)
	return
}

func (s *Server) getSecret(_ context.Context, name, version string, _ *azsecrets.GetSecretOptions) (resp azfake.Responder[azsecrets.GetSecretResponse], errResp azfake.ErrorResponder) {
	name, version = splitName(name, version)
	var (
		v   memstore.Version[secret]
		err error
	)
	if version == "" {
		v, err = s.store.Latest(name)
	} else if n, ok := versionNum(version); ok {
		v, err = s.store.At(name, n)
	} else {
		err = memstore.ErrNotFound
	}
	if err != nil {
		notFound(&errResp)
		return
	}
	resp.SetResponse(http.StatusOK, azsecrets.GetSecretResponse{Secret: bundle(name, v)}, nil)
	return
}

func (s *Server) deleteSecret(_ context.Context, name string, _ *azsecrets.DeleteSecretOptions) (resp azfake.Responder[azsecrets.DeleteSecretResponse], errResp azfake.ErrorResponder) {
	name, _ = splitName(name, "")
	v, err := s.store.Latest(name)
	if err != nil || s.store.Delete(name) != nil {
		notFound(&errResp)
		return
	}
	now := time.Now().UTC()
	resp.SetResponse(http.StatusOK, azsecrets.DeleteSecretResponse{DeletedSecret: azsecrets.DeletedSecret{
		ID: id(name, v.Num), Attributes: attrs(v.Value), DeletedDate: &now,
	}}, nil)
	return
}

func (s *Server) updateSecretProperties(_ context.Context, name, version string, p azsecrets.UpdateSecretPropertiesParameters, _ *azsecrets.UpdateSecretPropertiesOptions) (resp azfake.Responder[azsecrets.UpdateSecretPropertiesResponse], errResp azfake.ErrorResponder) {
	name, version = splitName(name, version)
	n := 0
	if version == "" {
		latest, err := s.store.Latest(name)
		if err != nil {
			notFound(&errResp)
			return
		}
		n = latest.Num
	} else if parsed, ok := versionNum(version); ok {
		n = parsed
	}
	err := s.store.Update(name, n, func(v *secret) {
		if p.ContentType != nil {
			v.contentType = p.ContentType
		}
		if p.Tags != nil {
			v.tags = p.Tags
		}
		if p.SecretAttributes != nil && p.SecretAttributes.Enabled != nil {
			v.enabled = p.SecretAttributes.Enabled
		}
		v.updated = time.Now().UTC()
	})
	if err != nil {
		notFound(&errResp)
		return
	}
	v, _ := s.store.At(name, n)
	resp.SetResponse(http.StatusOK, azsecrets.UpdateSecretPropertiesResponse{Secret: bundle(name, v)}, nil)
	return
}

func (s *Server) listSecrets(_ *azsecrets.ListSecretPropertiesOptions) (resp azfake.PagerResponder[azsecrets.ListSecretPropertiesResponse]) {
	var items []*azsecrets.SecretProperties
	for _, name := range s.store.Keys() {
		if v, err := s.store.Latest(name); err == nil {
			items = append(items, props(name, v))
		}
	}
	resp.AddPage(http.StatusOK, azsecrets.ListSecretPropertiesResponse{
		SecretPropertiesListResult: azsecrets.SecretPropertiesListResult{Value: items},
	}, nil)
	return
}

func (s *Server) listVersions(name string, _ *azsecrets.ListSecretPropertiesVersionsOptions) (resp azfake.PagerResponder[azsecrets.ListSecretPropertiesVersionsResponse]) {
	name, _ = splitName(name, "")
	vs, err := s.store.Versions(name)
	if err != nil {
		resp.AddResponseError(http.StatusNotFound, "SecretNotFound")
		return
	}
	var items []*azsecrets.SecretProperties
	for _, v := range vs {
		items = append(items, props(name, v))
	}
	resp.AddPage(http.StatusOK, azsecrets.ListSecretPropertiesVersionsResponse{
		SecretPropertiesListResult: azsecrets.SecretPropertiesListResult{Value: items},
	}, nil)
	return
}
