# minism

English | [日本語](README.ja.md)

Fake secret managers for Go tests. In memory. No network, no Docker, no account.

## Start (2 minutes)

1. Install the provider you use (AWS shown; others are [below](#pick-your-provider)):

   ```
   go get github.com/yashikota/minism/awssmtest
   ```

2. In a test, create the fake and take its client:

   ```go
   client := awssmtest.New(t).Client() // the normal *secretsmanager.Client, pointed at the fake

   client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
       Name: aws.String("prod/db"), SecretString: aws.String("s3cret"),
   })
   out, _ := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String("prod/db")})
   // *out.SecretString == "s3cret"
   ```

3. Run `go test`.

Your code keeps using the real AWS client. Only the server behind it is fake.

## Pick your provider

Each row is its own Go module. The package name is the last path segment.

**Big clouds**

| Provider | Install | Client you get |
|---|---|---|
| AWS Secrets Manager | `go get github.com/yashikota/minism/awssmtest` | `*secretsmanager.Client` |
| Google Secret Manager | `go get github.com/yashikota/minism/gcpsmtest` | `*secretmanager.Client` |
| Azure Key Vault (secrets) | `go get github.com/yashikota/minism/azsecretstest` | `*azsecrets.Client` |
| OCI Vault | `go get github.com/yashikota/minism/ocisecretstest` | `VaultsClient` + `SecretsClient` |
| IBM Cloud Secrets Manager | `go get github.com/yashikota/minism/ibmsmtest` | `*SecretsManagerV2` |

**Vault family and self-hosted**

| Provider | Install | Client you get |
|---|---|---|
| HashiCorp Vault | `go get github.com/yashikota/minism/vaulttest` | `*api.Client` |
| OpenBao | `go get github.com/yashikota/minism/openbaotest` | `*api.Client` |
| Infisical | `go get github.com/yashikota/minism/infisicaltest` | `infisical.InfisicalClientInterface` |

**Password and secret SaaS**

| Provider | Install | Client you get |
|---|---|---|
| 1Password | `go get github.com/yashikota/minism/onepasswordtest` | `*onepassword.Client` |
| Keeper Secrets Manager | `go get github.com/yashikota/minism/keepertest` | `*core.SecretsManager` |
| Akeyless | `go get github.com/yashikota/minism/akeylesstest` | `*akeyless.V2ApiService` |
| Cloudflare Secrets Store | `go get github.com/yashikota/minism/cfsecretstest` | `*cloudflare.Client` |

Every provider has a README next to its code (for example [awssmtest](awssmtest/README.md)) with
what it supports, what it does not, and a runnable snippet.

## Good to know

- **Unsupported calls fail loudly.** You get `minism: <provider> <operation> not implemented`, never a silent success.
- **No auth check.** Any credentials or token work.
- **Fresh server per `New(t)`.** Nothing is shared between tests; it is discarded when the test ends.
- **`Value(...)` and `Seed(...)` are test helpers**, not part of the provider's API. Use them to set up and assert only.
- **These are not the real services.** They cover create / read / update / delete / list / versions. No IAM, rotation, replication or KMS.

## If a call fails

1. Message contains `not implemented`: the operation is not supported. Open the provider's README, section "Supported".
2. Anything else: it is the real client reporting a real error from the fake (not found, already exists, ...). Check your test's setup.

<details>
<summary>Full example: app code and its test (5 minutes)</summary>

Make your code depend on a small interface, then pass the minism client in tests.

`app.go`, with nothing minism-specific:

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

`app_test.go`: create what the code expects, then run the code.

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
    client := awssmtest.New(t).Client()

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
    // Empty server: the real client returns a real ResourceNotFoundException.
    if _, err := myapp.LoadDBPassword(context.Background(), awssmtest.New(t).Client()); err == nil {
        t.Fatal("expected an error")
    }
}
```

</details>

<details>
<summary>How it works</summary>

```
your code ──► real SDK client ──► minism transport ──► in-memory server
```

The transport depends on what each SDK offers: an injectable `http.Client` (AWS, IBM, OCI,
Akeyless, Cloudflare, Vault, OpenBao), a gRPC connection (GCP), a vendor-provided fake (Azure),
exported API fields (1Password), public interfaces (Infisical), or a test-only transport hook
(Keeper). Nothing opens a socket. CI runs every module in a network namespace with no route out.

</details>

<details>
<summary>Development</summary>

Shared code is in the root module under `internal/`: `memstore` (versioned in-memory store),
`rtfake` (turns an `http.Handler` into a `RoundTripper`), `kvfake` (the Vault-compatible KV protocol
shared by `vaulttest` and `openbaotest`).

`go.work` ties the modules together locally. CI builds each module alone (`GOWORK=off`), vets it,
then runs its tests with the network cut off (`unshare --net`, `GOPROXY=off`).

```
for d in $(find . -name go.mod -exec dirname {} \;); do (cd $d && GOWORK=off go test -race ./...); done
```

Versions are tags: `v0.1.0` for the root module, `awssmtest/v0.1.0` and so on per provider.
If you change `internal/`, tag the root first, then update each module's `require`.

One upstream bug is worked around: `azsecrets/fake` v1.5.0 mis-parses `/secrets/{name}/{version}`
(see [azsecretstest](azsecretstest/README.md)).

</details>
