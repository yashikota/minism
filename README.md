# minism

Test doubles for secret managers, for Go. Your code under test keeps using the
**real official SDK client**; minism swaps what is on the other end of the wire for an
in-memory server. No network, no Docker, no external process, nothing to clean up.

```go
client := awssmtest.New(t).Client() // a real *secretsmanager.Client, backed by memory
```

Because the client is the real one, request building, signing, retries, error types and
pagination in your code are exercised for real. Only the server is fake.

## Pick your provider

| Provider | Import | You get | Backend | README |
|---|---|---|---|---|
| AWS Secrets Manager | `awssmtest` | `*secretsmanager.Client` | our in-memory endpoint | [awssmtest](awssmtest/README.md) |
| Google Secret Manager | `gcpsmtest` | `*secretmanager.Client` | our in-memory gRPC server | [gcpsmtest](gcpsmtest/README.md) |
| Azure Key Vault (secrets) | `azsecretstest` | `*azsecrets.Client` | Microsoft's own `azsecrets/fake` | [azsecretstest](azsecretstest/README.md) |
| HashiCorp Vault | `vaulttest` | `*api.Client` | our in-memory KV server | [vaulttest](vaulttest/README.md) |
| OpenBao | `openbaotest` | `*api.Client` | same KV server as Vault | [openbaotest](openbaotest/README.md) |
| 1Password | `onepasswordtest` | `*onepassword.Client` | in-memory APIs injected into the client | [onepasswordtest](onepasswordtest/README.md) |
| Infisical | `infisicaltest` | `infisical.InfisicalClientInterface` | in-memory implementation of the SDK interfaces | [infisicaltest](infisicaltest/README.md) |
| OCI Vault | `ocisecretstest` | `VaultsClient` + `SecretsClient` | our in-memory endpoint | [ocisecretstest](ocisecretstest/README.md) |
| IBM Cloud Secrets Manager | `ibmsmtest` | `*SecretsManagerV2` | our in-memory endpoint | [ibmsmtest](ibmsmtest/README.md) |
| Akeyless | `akeylesstest` | `*akeyless.V2ApiService` | our in-memory endpoint | [akeylesstest](akeylesstest/README.md) |
| Cloudflare Secrets Store | `cfsecretstest` | `*cloudflare.Client` | our in-memory endpoint | [cfsecretstest](cfsecretstest/README.md) |
| Keeper Secrets Manager | `keepertest` | `*core.SecretsManager` | our in-memory encrypted endpoint | [keepertest](keepertest/README.md) |

Each provider is a separate Go module, so depending on one pulls in only that provider's SDK.
The package name is the last path segment (`awssmtest`, `gcpsmtest`, ...).

## Using it

The pattern is the same for every provider. Make your code depend on a small interface (or on the
SDK client type), then give it the minism client in tests.

**Your code** (`app.go`): nothing minism-specific.

```go
package myapp

import (
    "context"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// The slice of the AWS client this app uses. *secretsmanager.Client satisfies it.
type SecretsAPI interface {
    GetSecretValue(ctx context.Context, in *secretsmanager.GetSecretValueInput,
        opts ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

func LoadDBPassword(ctx context.Context, c SecretsAPI) (string, error) {
    out, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String("prod/db")})
    if err != nil {
        return "", err
    }
    return aws.ToString(out.SecretString), nil
}
```

**Your test** (`app_test.go`): create what the code expects, then run the code.

```go
package myapp_test

import (
    "context"
    "testing"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
    "github.com/yashikota/minism/awssmtest"

    myapp "example.com/myapp"
)

func TestLoadDBPassword(t *testing.T) {
    ctx := context.Background()
    client := awssmtest.New(t).Client() // fresh in-memory AWS Secrets Manager

    _, err := client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
        Name: aws.String("prod/db"), SecretString: aws.String("s3cret"),
    })
    if err != nil {
        t.Fatal(err)
    }

    got, err := myapp.LoadDBPassword(ctx, client)
    if err != nil || got != "s3cret" {
        t.Fatalf("got %q, %v", got, err)
    }
}

func TestMissingSecret(t *testing.T) {
    // An empty server: the real client returns a real ResourceNotFoundException.
    if _, err := myapp.LoadDBPassword(context.Background(), awssmtest.New(t).Client()); err == nil {
        t.Fatal("expected an error")
    }
}
```

That is the whole recipe: `New(t)` for a fresh server, `.Client()` for the official client,
then create the state you need through the SDK itself. Other providers work the same way;
their READMEs have the equivalent snippet.

### Adding it to your project today

Modules are not tagged yet, so `go get` cannot resolve them. Until they are, clone this repo and
point your `go.mod` at it (this setup is tested):

```
git clone https://github.com/yashikota/minism ../minism
```

```
// go.mod of your project
require github.com/yashikota/minism/awssmtest v0.0.0

replace github.com/yashikota/minism/awssmtest => ../minism/awssmtest
replace github.com/yashikota/minism          => ../minism
```

Then `go mod tidy`. Use one `replace` line per provider you import, plus the last line (the shared
root module) once. Once the modules are tagged you will instead run
`go get github.com/yashikota/minism/awssmtest`.

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
