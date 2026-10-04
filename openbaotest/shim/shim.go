// Package shim exposes OpenBao's internal in-memory test core.
//
// OpenBao keeps its whole implementation under internal/, and Go decides
// "internal" visibility purely from the importer's import path. This module is
// therefore declared as github.com/openbao/openbao/v2/shim, which makes it a
// legitimate importer of internal/vault, internal/http and the real KV engine.
// It adds no behaviour of its own: it only re-exports a constructor.
package shim

import (
	"net/http"
	"testing"

	"github.com/openbao/openbao/sdk/v2/logical"
	"github.com/openbao/openbao/v2/internal/builtin/logical/kv"
	"github.com/openbao/openbao/v2/internal/helper/configutil"
	ohttp "github.com/openbao/openbao/v2/internal/http"
	"github.com/openbao/openbao/v2/internal/vault"
)

// New boots a pure in-memory OpenBao core (physical/inmem) with the real KV
// engine registered, and returns OpenBao's real HTTP handler plus the root token.
func New(t *testing.T) (http.Handler, string) {
	t.Helper()
	core, _, token := vault.TestCoreUnsealedWithConfig(t, &vault.CoreConfig{
		LogicalBackends: map[string]logical.Factory{"kv": kv.Factory},
	})
	h := ohttp.Handler.Handler(&vault.HandlerProperties{
		Core:           core,
		ListenerConfig: &configutil.Listener{},
	})
	return h, token
}
