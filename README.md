# minism

In-memory secret-manager providers for Go tests. Each package hands back the **real
official client** (or the SDK's own client interface) wired to an in-memory backend: no
network, no listener, no external process. CI runs every module inside a network
namespace with no route out to prove it.

```go
c := awssmtest.New(t).Client() // *secretsmanager.Client, the real one
```

Every package has the same shape: `New(t)`, `Client()` (or `Env()` for OCI), plus
fixture helpers (`Value`, `Seed`, ...) that are *not* part of the provider contract.
Unsupported operations fail loudly (`minism: <provider> <op> not implemented`) instead
of silently succeeding.

## Providers

Rule: use the upstream implementation if one exists; otherwise a real client plus the
SDK's own test seam; write server-side state only when upstream has none.

| Module | Real client | Backend | Own code |
|---|---|---|---|
| `vaulttest` | `vault/api` | **real Vault core** + real KV, `inmem` storage, real HTTP handler | none |
| `openbaotest` | `openbao/api/v2` | **real OpenBao core** + real KV via a shim module | none |
| `azsecretstest` | `azsecrets.Client` | Microsoft's `azsecrets/fake` | state |
| `awssmtest` | `secretsmanager.Client` | `aws.Config.HTTPClient` | AWS JSON endpoint |
| `gcpsmtest` | `secretmanager.Client` | bufconn + generated `SecretManagerServiceServer` | state |
| `onepasswordtest` | `onepassword.Client` | exported `ItemsAPI`/`VaultsAPI`/`SecretsAPI` fields | state |
| `infisicaltest` | `InfisicalClientInterface` | SDK's own interfaces | state |
| `ocisecretstest` | `vault.VaultsClient` + `secrets.SecretsClient` | `HTTPRequestDispatcher` | REST endpoint |
| `ibmsmtest` | `SecretsManagerV2` | `SetHTTPClient` | REST endpoint |
| `akeylesstest` | `akeyless.V2ApiService` | `Configuration.HTTPClient` | RPC endpoint |
| `cfsecretstest` | `cloudflare.Client` | `option.WithHTTPClient` | REST endpoint |
| `keepertest` | `core.SecretsManager` | SDK's test-only `Context.Transport` | encrypted endpoint |

Shared internals (`internal/memstore`, `internal/rtfake`) live in the root module.

## Things that differ from the original plan

- **Vault** has no `InmemNetworkListener` (v1.21.4 still uses `net.ListenTCP`). `vaulttest`
  boots `vault.TestCoreUnsealedWithConfig` and serves the real handler through a
  RoundTripper instead, which needs no socket and no temp dir.
- **OpenBao** is `package main` plus `internal/`, so `//go:linkname` has nothing to link
  against. Go decides `internal` visibility from the importer's *import path only*, so
  `openbaotest/shim` is declared as `github.com/openbao/openbao/v2/shim` and imports the
  internals directly. No linkname, no `unsafe`.
- **Vault and OpenBao** modules cannot be built from tagged versions alone (their `go.mod`s
  use `replace` for their own `sdk`/`api`, which does not propagate). Their READMEs list
  the `replace` lines a consumer must copy.
- **azsecrets/fake v1.5.0** mis-parses `/secrets/{name}/{version}` (a `$-;` character range
  swallows `/`). `azsecretstest` splits the name again; it is a no-op once upstream is fixed.

## Development

Each provider is its own Go module (so depending on one does not pull in the others).
`go.work` ties them together locally; CI builds each module standalone with `GOWORK=off`,
vets it, pre-fetches dependencies, then runs the tests with the network cut off
(`unshare --net`, `GOPROXY=off`).

```
for d in $(find . -name go.mod -exec dirname {} \;); do (cd $d && GOWORK=off go test -race ./...); done
```
