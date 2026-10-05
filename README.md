# minism

Test doubles for secret managers, for Go. Your code under test keeps using the
**real official SDK client**; minism swaps what is on the other end of the wire for an
in-memory server. No network, no Docker, no external process, nothing to clean up.

```go
func TestLoadsDBPassword(t *testing.T) {
    srv := awssmtest.New(t)                  // in-memory AWS Secrets Manager
    client := srv.Client()                   // a real *secretsmanager.Client

    client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
        Name: aws.String("prod/db"), SecretString: aws.String("s3cret"),
    })

    got := LoadDBPassword(ctx, client)       // your code, unchanged
    if got != "s3cret" { t.Fatal(got) }
}
```

Because the client is the real one, request building, signing, retries, error types and
pagination in your code are exercised for real. Only the server is fake.

## Pick your provider

| Provider | Install | You get | Backend | README |
|---|---|---|---|---|
| AWS Secrets Manager | `go get github.com/yashikota/minism/awssmtest` | `*secretsmanager.Client` | our in-memory endpoint | [awssmtest](awssmtest/README.md) |
| Google Secret Manager | `go get github.com/yashikota/minism/gcpsmtest` | `*secretmanager.Client` | our in-memory gRPC server | [gcpsmtest](gcpsmtest/README.md) |
| Azure Key Vault (secrets) | `go get github.com/yashikota/minism/azsecretstest` | `*azsecrets.Client` | Microsoft's own `azsecrets/fake` | [azsecretstest](azsecretstest/README.md) |
| HashiCorp Vault | `go get github.com/yashikota/minism/vaulttest` | `*api.Client` | our in-memory KV server | [vaulttest](vaulttest/README.md) |
| OpenBao | `go get github.com/yashikota/minism/openbaotest` | `*api.Client` | same KV server as Vault | [openbaotest](openbaotest/README.md) |
| 1Password | `go get github.com/yashikota/minism/onepasswordtest` | `*onepassword.Client` | in-memory APIs injected into the client | [onepasswordtest](onepasswordtest/README.md) |
| Infisical | `go get github.com/yashikota/minism/infisicaltest` | `infisical.InfisicalClientInterface` | in-memory implementation of the SDK interfaces | [infisicaltest](infisicaltest/README.md) |
| OCI Vault | `go get github.com/yashikota/minism/ocisecretstest` | `VaultsClient` + `SecretsClient` | our in-memory endpoint | [ocisecretstest](ocisecretstest/README.md) |
| IBM Cloud Secrets Manager | `go get github.com/yashikota/minism/ibmsmtest` | `*SecretsManagerV2` | our in-memory endpoint | [ibmsmtest](ibmsmtest/README.md) |
| Akeyless | `go get github.com/yashikota/minism/akeylesstest` | `*akeyless.V2ApiService` | our in-memory endpoint | [akeylesstest](akeylesstest/README.md) |
| Cloudflare Secrets Store | `go get github.com/yashikota/minism/cfsecretstest` | `*cloudflare.Client` | our in-memory endpoint | [cfsecretstest](cfsecretstest/README.md) |
| Keeper Secrets Manager | `go get github.com/yashikota/minism/keepertest` | `*core.SecretsManager` | our in-memory encrypted endpoint | [keepertest](keepertest/README.md) |

Each provider is a separate Go module, so depending on one pulls in only that provider's SDK.
The package name is the last path segment (`awssmtest`, `gcpsmtest`, ...).

> **Status: pre-release.** Modules are not tagged yet, so `go get` will not resolve until they
> are. Until then, clone the repo and use the `go.work` workspace.

## What "fake" means here

These are **not** the real services and do not try to be. Each one implements the common
operations tests need (create / read / update / delete / list / versions) and nothing else.
No IAM, no rotation, no replication, no KMS, no quotas.

- **Unsupported operations fail loudly.** You get an error saying
  `minism: <provider> <operation> not implemented` (or the SDK's own "not implemented" error)
  instead of a silent success. If you hit one, that is a gap to fill, not a bug in your code.
- **Authentication is not checked.** Any credentials or token are accepted.
- **Each `New(t)` is a fresh, isolated server.** State is not shared between tests, and it is
  discarded with the test.
- **Fixture helpers** such as `Value(...)` or `Seed(...)` let a test look at or preload the
  server's state directly. They are *not* part of the provider's API; use them for setup and
  assertions only.

Every provider README lists exactly what is supported and what is not.

## How it works

```
your code ──► real SDK client ──► minism transport ──► in-memory server
              (signing, retries,   (a RoundTripper,       (a small state machine
               error types)         bufconn, or the        per provider)
                                    SDK's own test seam)
```

How the client is redirected depends on what each SDK offers: an injectable
`http.Client` (AWS, IBM, OCI, Akeyless, Cloudflare, Vault, OpenBao), a gRPC connection (GCP),
a vendor-provided fake (Azure), exported API fields (1Password), public interfaces (Infisical),
or a test-only transport hook (Keeper). Nothing opens a socket; CI runs every module inside a
network namespace with no route out to prove it.

## Development

Shared code lives in the root module under `internal/`: `memstore` (versioned in-memory
store), `rtfake` (turns an `http.Handler` into a `RoundTripper`) and `kvfake` (the
Vault-compatible KV protocol shared by `vaulttest` and `openbaotest`).

`go.work` ties the modules together locally. CI builds each module standalone
(`GOWORK=off`), vets it, pre-fetches dependencies, then runs its tests with the network cut off
(`unshare --net`, `GOPROXY=off`).

```
for d in $(find . -name go.mod -exec dirname {} \;); do (cd $d && GOWORK=off go test -race ./...); done
```

One known upstream bug is worked around: `azsecrets/fake` v1.5.0 mis-parses
`/secrets/{name}/{version}`; see [azsecretstest](azsecretstest/README.md).
