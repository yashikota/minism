# azsecretstest

**Azure Key Vault secrets** for tests. `Client()` returns a real `*azsecrets.Client` wired to
Microsoft's own `azsecrets/fake` server transport. The Key Vault REST protocol is implemented by
Microsoft; this package only supplies the in-memory store behind it.

## Install

```
go get github.com/yashikota/minism/azsecretstest
```

## Usage

```go
srv := azsecretstest.New(t)
c := srv.Client()

c.SetSecret(ctx, "my-db-password", azsecrets.SetSecretParameters{Value: to.Ptr("one")}, nil)
c.SetSecret(ctx, "my-db-password", azsecrets.SetSecretParameters{Value: to.Ptr("two")}, nil)

got, _ := c.GetSecret(ctx, "my-db-password", "", nil) // "" = latest
// *got.Value == "two"
```

## Supported operations

| Operation | Notes |
|---|---|
| `SetSecret` | every call adds a version; content type, tags, enabled flag kept |
| `GetSecret` | empty version = latest, otherwise a version id returned earlier |
| `UpdateSecretProperties` | content type, tags, enabled flag of one version |
| `DeleteSecret` | removes the secret outright (no soft-delete state) |
| `NewListSecretPropertiesPager` | latest version of each secret, one page |
| `NewListSecretPropertiesVersionsPager` | all versions of one secret, one page |

Missing secrets give a real `*azcore.ResponseError` with status 404 and code `SecretNotFound`.

## Not supported

Backup / restore, and the deleted-secret operations (`GetDeletedSecret`, `RecoverDeletedSecret`,
`PurgeDeletedSecret`, list deleted). The official fake answers
`fake for method <X> not implemented`.

## Test helpers (not part of the Azure API)

- `srv.Value(name) (string, bool)`: latest value.
- `srv.Len() int`: number of live secrets.

## Notes

- **Upstream bug worked around.** `azsecrets/fake` v1.5.0 builds its path regexp with the character
  range `$-;`, which includes `/`, so `/secrets/{name}/{version}` arrives with the version glued
  onto the name. Every handler here splits the name at the first `/` (Key Vault names never contain
  one). It becomes a no-op once upstream is fixed.
- Vault URL is `https://minism.vault.azure.net`; credentials are `azfake.TokenCredential`.
