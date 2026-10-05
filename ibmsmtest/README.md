# ibmsmtest

In-memory **IBM Cloud Secrets Manager** for tests. `Client()` returns a real
`*secretsmanagerv2.SecretsManagerV2` whose HTTP client is an in-memory REST server.

## Install

```
go get github.com/yashikota/minism/ibmsmtest
```

## Usage

```go
srv := ibmsmtest.New(t)
c := srv.Client()

res, _, _ := c.CreateSecret(&secretsmanagerv2.CreateSecretOptions{
    SecretPrototype: &secretsmanagerv2.ArbitrarySecretPrototype{
        Name: core.StringPtr("db-pass"), SecretType: core.StringPtr("arbitrary"), Payload: core.StringPtr("one"),
    },
})
secret := res.(*secretsmanagerv2.ArbitrarySecret)

got, _, _ := c.GetSecret(&secretsmanagerv2.GetSecretOptions{ID: secret.ID})
// *got.(*secretsmanagerv2.ArbitrarySecret).Payload == "one"
```

## Supported

Only **arbitrary** secrets.

| Operation | Notes |
|---|---|
| `CreateSecret` | same name in the same group → 409; group defaults to `default` |
| `GetSecret`, `GetSecretMetadata` | |
| `GetSecretByNameType` | type must be `arbitrary` |
| `ListSecrets` | `offset`, `limit`, `search` (name substring); returns `next` links |
| `UpdateSecretMetadata` | name, description, labels, custom metadata |
| `DeleteSecret` | |
| `CreateSecretVersion`, `ListSecretVersions` (newest first), `GetSecretVersion` | version id, or the aliases `current` / `previous` |

Errors carry the real response, so `resp.StatusCode` is 404 / 409 as IBM returns.

## Not supported

Other secret types (`kv`, `iam_credentials`, certificates, ...), secret groups, locks, tasks,
rotation, actions, version metadata updates. Unsupported secret types answer 501 on create; other
unsupported routes answer 501 `not_implemented`.

## Test helpers (not part of the IBM API)

- `srv.Payload(name) (string, bool)`: current payload of the secret with that name.

## Notes

- No authentication (`core.NoAuthAuthenticator`); service URL is fixed and never dialled.
