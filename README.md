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

Rule: use an upstream-provided implementation if one exists (only Azure does); otherwise a real client plus the
SDK's own test seam; write server-side state only when upstream has none.

| Module | Real client | Backend | Own code |
|---|---|---|---|
| `vaulttest` | `vault/api` | `kvfake` (KV v1/v2, sys/mounts) | wire protocol, shared |
| `openbaotest` | `openbao/api/v2` | `kvfake`, same handler | wire protocol, shared |
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

Shared internals (`internal/memstore`, `internal/rtfake`, `internal/kvfake`) live in the root module.

## Notes on design choices

- **Vault / OpenBao are not run for real.** Their cores cannot be imported cleanly (their
  `go.mod`s use `replace` for their own `sdk`/`api`, which does not propagate, so every
  consumer would have to copy `replace` lines), and OpenBao is `package main` plus
  `internal/`. We use the lightweight official `api` clients against a small KV
  implementation of the shared wire protocol instead (`internal/kvfake`).
- **azsecrets/fake v1.5.0** mis-parses `/secrets/{name}/{version}` (a `$-;` character range
  swallows `/`). `azsecretstest` splits the name again; it is a no-op once upstream is fixed.
- Anything a fake does not implement fails with `minism: <provider> <op> not implemented`
  rather than silently succeeding.

## Development

Each provider is its own Go module (so depending on one does not pull in the others).
`go.work` ties them together locally; CI builds each module standalone with `GOWORK=off`,
vets it, pre-fetches dependencies, then runs the tests with the network cut off
(`unshare --net`, `GOPROXY=off`).

```
for d in $(find . -name go.mod -exec dirname {} \;); do (cd $d && GOWORK=off go test -race ./...); done
```
