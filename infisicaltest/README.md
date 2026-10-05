# infisicaltest

In-memory **Infisical** for tests. Unlike the other providers, this returns an
`infisical.InfisicalClientInterface` rather than the concrete client: the Infisical Go SDK keeps its
API fields private, so its public interfaces are the supported seam. Code that depends on those
interfaces (or on `SecretsInterface` via `client.Secrets()`) works unchanged.

## Install

```
go get github.com/yashikota/minism/infisicaltest
```

Not tagged yet, so `go get` will not resolve for now: see [Adding it to your project today](../README.md#adding-it-to-your-project-today).

## Usage

```go
srv := infisicaltest.New(t)
c := srv.Client() // infisical.InfisicalClientInterface

c.Secrets().Create(infisical.CreateSecretOptions{
    ProjectID: "proj", Environment: "dev", SecretKey: "DB", SecretValue: "one",
})
c.Secrets().Update(infisical.UpdateSecretOptions{
    ProjectID: "proj", Environment: "dev", SecretKey: "DB", NewSecretValue: "two",
})

s, _ := c.Secrets().Retrieve(infisical.RetrieveSecretOptions{
    ProjectID: "proj", Environment: "dev", SecretKey: "DB",
})
// s.SecretValue == "two", s.Version == 2
```

## Supported

| Interface | Operations |
|---|---|
| `Secrets()` | `Create`, `Retrieve` (latest or `Version`), `Update` (adds a version), `Delete`, `List` / `ListSecrets` (by path, optionally `Recursive`; returns an ETag), `Batch().Create` |
| `Folders()` | `Create`, `List`, `Update` (rename), `Delete` (by ID or name) |
| `Auth()` | every login method succeeds and returns a fake token; `SetAccessToken` / `GetAccessToken` / `RevokeAccessToken` work |

Secrets are addressed by project + environment + path + key. Errors are the SDK's own
`*infisical.APIError` with status 404 (missing) or 400 (duplicate), so `errors.As` works.

## Not supported

- **`DynamicSecrets()`, `Kms()` and `Ssh()` panic** if used.
- Secret imports, tags, metadata, `IfNoneMatch` (ignored), secret references expansion.

## Test helpers (not part of the Infisical API)

- `srv.Value(project, env, path, key) (string, bool)`: latest value.

## Notes

- A project is matched by whatever string you pass as `ProjectID` (or `ProjectSlug`); there is no mapping between the two, so use one consistently.
- An empty path means `/`.
