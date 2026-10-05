# ocisecretstest

In-memory **OCI Vault secrets** for tests. OCI splits secrets across two official clients;
`Env()` returns both, sharing one in-memory state:

- `vault.VaultsClient`: manage secrets (create, update, delete, versions).
- `secrets.SecretsClient`: read secret bundles (the actual values).

Requests are really signed with a generated RSA key (the signature is not verified).

## Install

```
go get github.com/yashikota/minism/ocisecretstest
```

## Usage

```go
srv := ocisecretstest.New(t)
env := srv.Env() // env.Vault, env.Secrets, env.CompartmentID, env.VaultID, env.KeyID

created, _ := env.Vault.CreateSecret(ctx, vault.CreateSecretRequest{CreateSecretDetails: vault.CreateSecretDetails{
    CompartmentId: &env.CompartmentID, VaultId: &env.VaultID, KeyId: &env.KeyID,
    SecretName:    common.String("db-pass"),
    SecretContent: vault.Base64SecretContentDetails{Content: common.String(base64.StdEncoding.EncodeToString([]byte("one")))},
}})

bundle, _ := env.Secrets.GetSecretBundle(ctx, secrets.GetSecretBundleRequest{SecretId: created.Id})
// bundle.SecretBundleContent is a secrets.Base64SecretBundleContentDetails holding "one" (base64)
```

## Supported

| Client | Operations |
|---|---|
| `Vault` | `CreateSecret` (duplicate name in a vault → 409), `GetSecret`, `UpdateSecret` (new `SecretContent` adds a version; description, freeform tags), `ListSecrets` (filters: compartment, name, vault, lifecycle state), `ListSecretVersions`, `GetSecretVersion`, `ScheduleSecretDeletion`, `CancelSecretDeletion` |
| `Secrets` | `GetSecretBundle`, `GetSecretBundleByName`, by `VersionNumber`, `Stage` (`CURRENT`, `LATEST`, `PREVIOUS`) or version name |

Errors are real `common.ServiceError`s (`common.IsServiceError` works): 404
`NotAuthorizedOrNotFound`, 409 `Conflict`.

## Not supported

Rotation, per-version scheduled deletion, moving between compartments, secret rules, replication,
list pagination. They answer 501 `NotImplemented`.

## Test helpers (not part of the OCI API)

- `srv.Content(name) (string, bool)`: base64 content of the `CURRENT` version.

## Notes

- Content is base64, exactly as OCI stores and returns it.
- A secret scheduled for deletion cannot be read as a bundle, and is never actually purged.
- The IDs in `Env` (`CompartmentID`, `VaultID`, `KeyID`) are not validated; they are only there so you don't have to invent them.
