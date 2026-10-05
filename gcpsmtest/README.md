# gcpsmtest

In-memory **Google Cloud Secret Manager** for tests. `Client()` returns a real
`*secretmanager.Client` connected over an in-memory gRPC connection (`bufconn`) to a small
implementation of Google's generated `SecretManagerServiceServer`.

## Install

```
go get github.com/yashikota/minism/gcpsmtest
```

## Usage

```go
srv := gcpsmtest.New(t)
c := srv.Client()

c.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{Parent: "projects/p", SecretId: "db-pass"})
c.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
    Parent:  "projects/p/secrets/db-pass",
    Payload: &secretmanagerpb.SecretPayload{Data: []byte("one")},
})

res, _ := c.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
    Name: "projects/p/secrets/db-pass/versions/latest",
})
// string(res.Payload.Data) == "one"
```

## Supported operations

| RPC | Notes |
|---|---|
| `CreateSecret` | duplicate → `AlreadyExists` |
| `GetSecret`, `UpdateSecret` | `UpdateSecret` handles the `labels` mask path |
| `ListSecrets` | all secrets under the parent, sorted; no filter, no paging |
| `DeleteSecret` | removes the secret and all its versions |
| `AddSecretVersion` | verifies `data_crc32c` when you send it |
| `GetSecretVersion`, `AccessSecretVersion` | version number or `latest`; access needs state `ENABLED` (else `FailedPrecondition`); returns `data_crc32c` |
| `ListSecretVersions` | newest first |
| `DisableSecretVersion`, `EnableSecretVersion`, `DestroySecretVersion` | destroy drops the payload |

Errors are real gRPC status codes (`NotFound`, `AlreadyExists`, `FailedPrecondition`, ...).

## Not supported

Anything else returns gRPC `Unimplemented` (the generated base server): IAM, tags, topics,
rotation, annotations, regional endpoints, etc.

## Test helpers (not part of the GCP API)

- `srv.Value(secretName) ([]byte, bool)`: payload of the newest `ENABLED` version. `secretName` is the full `projects/p/secrets/id`.

## Notes

- The project in `parent` is not validated; any `projects/<x>` works.
- The client and its connection are closed automatically at the end of the test.
