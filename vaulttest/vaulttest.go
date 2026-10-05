// Package vaulttest gives tests a real Vault *api.Client whose HTTP layer is an
// in-memory Vault-compatible KV server (sys/mounts, KV v1 and v2).
//
// It is a minimal reimplementation of the wire protocol, not Vault itself: there
// is no auth, policy, lease or seal, and any token is accepted. Unsupported paths
// answer 501.
package vaulttest

import (
	"testing"

	"github.com/hashicorp/vault/api"

	"github.com/yashikota/minism/internal/kvfake"
	"github.com/yashikota/minism/internal/rtfake"
)

const token = "root"

// Server is an in-memory Vault.
type Server struct {
	t  testing.TB
	kv *kvfake.Server
}

// New starts the server with a kv-v2 engine mounted at "secret/", like a dev server.
func New(t testing.TB) *Server {
	t.Helper()
	return &Server{t: t, kv: kvfake.New()}
}

// Client returns a new official Vault client pointed at the in-memory server.
func (s *Server) Client() *api.Client {
	s.t.Helper()
	cfg := api.DefaultConfig()
	cfg.Address = "http://vault.invalid"
	cfg.HttpClient = rtfake.Client(s.kv)
	cfg.MaxRetries = 0
	c, err := api.NewClient(cfg)
	if err != nil {
		s.t.Fatalf("vaulttest: new client: %v", err)
	}
	c.SetToken(token)
	return c
}

// Token is the token the client is configured with (any token is accepted).
func (s *Server) Token() string { return token }
