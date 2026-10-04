// Package vaulttest runs the real HashiCorp Vault core, in memory, for tests.
//
// There is no fake here: it boots vault.TestCoreUnsealedWithConfig (physical
// storage is Vault's own inmem backend), mounts the real KV plugin, and wires
// the official *api.Client straight to Vault's real HTTP handler through a
// RoundTripper. No listener is opened and no port is used.
//
// Vault is licensed under BUSL-1.1; see the README before depending on this.
package vaulttest

import (
	"net/http"
	"testing"

	kv "github.com/hashicorp/vault-plugin-secrets-kv"
	"github.com/hashicorp/vault/api"
	vaulthttp "github.com/hashicorp/vault/http"
	"github.com/hashicorp/vault/internalshared/configutil"
	"github.com/hashicorp/vault/sdk/logical"
	"github.com/hashicorp/vault/vault"

	"github.com/yashikota/minism/internal/rtfake"
)

// Server is a running in-memory Vault.
type Server struct {
	t       testing.TB
	handler http.Handler
	token   string
}

// New starts Vault with a kv-v2 engine mounted at "secret/", like `vault server -dev`.
func New(t testing.TB) *Server {
	t.Helper()
	core, _, token := vault.TestCoreUnsealedWithConfig(t, &vault.CoreConfig{
		LogicalBackends: map[string]logical.Factory{"kv": kv.Factory},
	})
	s := &Server{
		t:     t,
		token: token,
		handler: vaulthttp.Handler.Handler(&vault.HandlerProperties{
			Core:           core,
			ListenerConfig: &configutil.Listener{},
		}),
	}

	// TestCore mounts kv-v1 at secret/; replace it with v2.
	c := s.Client()
	if err := c.Sys().Unmount("secret"); err != nil {
		t.Fatalf("vaulttest: unmount secret: %v", err)
	}
	err := c.Sys().Mount("secret", &api.MountInput{Type: "kv", Options: map[string]string{"version": "2"}})
	if err != nil {
		t.Fatalf("vaulttest: mount kv-v2: %v", err)
	}
	return s
}

// Client returns a new official Vault client authenticated with the root token.
func (s *Server) Client() *api.Client {
	s.t.Helper()
	cfg := api.DefaultConfig()
	cfg.Address = "http://vault.invalid"
	cfg.HttpClient = rtfake.Client(s.handler)
	cfg.MaxRetries = 0
	c, err := api.NewClient(cfg)
	if err != nil {
		s.t.Fatalf("vaulttest: new client: %v", err)
	}
	c.SetToken(s.token)
	return c
}

// Token is the root token.
func (s *Server) Token() string { return s.token }
