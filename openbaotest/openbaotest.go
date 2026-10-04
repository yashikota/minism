// Package openbaotest runs the real OpenBao core, in memory, for tests.
//
// The official *api.Client talks to OpenBao's real HTTP handler through a
// RoundTripper; no listener or port is involved. See README for the go.mod
// replace line this package needs.
package openbaotest

import (
	"net/http"
	"testing"

	"github.com/openbao/openbao/api/v2"
	"github.com/openbao/openbao/v2/shim"

	"github.com/yashikota/minism/internal/rtfake"
)

// Server is a running in-memory OpenBao.
type Server struct {
	t       testing.TB
	handler http.Handler
	token   string
}

// New starts OpenBao with a kv-v2 engine at "secret/", like `bao server -dev`.
func New(t *testing.T) *Server {
	t.Helper()
	h, token := shim.New(t)
	s := &Server{t: t, handler: h, token: token}

	// The test core mounts kv-v1 at secret/; replace it with v2.
	c := s.Client()
	if err := c.Sys().Unmount("secret"); err != nil {
		t.Fatalf("openbaotest: unmount secret: %v", err)
	}
	err := c.Sys().Mount("secret", &api.MountInput{Type: "kv", Options: map[string]string{"version": "2"}})
	if err != nil {
		t.Fatalf("openbaotest: mount kv-v2: %v", err)
	}
	return s
}

// Client returns a new official OpenBao client authenticated with the root token.
func (s *Server) Client() *api.Client {
	s.t.Helper()
	cfg := api.DefaultConfig()
	cfg.Address = "http://openbao.invalid"
	cfg.HttpClient = rtfake.Client(s.handler)
	cfg.MaxRetries = 0
	c, err := api.NewClient(cfg)
	if err != nil {
		s.t.Fatalf("openbaotest: new client: %v", err)
	}
	c.SetToken(s.token)
	return c
}

// Token is the root token.
func (s *Server) Token() string { return s.token }
