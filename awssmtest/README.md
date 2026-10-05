# awssmtest

In-memory **AWS Secrets Manager** for tests. `Client()` returns a real
`*secretsmanager.Client` (aws-sdk-go-v2); its serializer, signer and middleware all run, and only
the HTTP call is answered in memory.

## Install

```
go get github.com/yashikota/minism/awssmtest
```

Not tagged yet, so `go get` will not resolve for now: see [Adding it to your project today](../README.md#adding-it-to-your-project-today).

## Usage

```go
srv := awssmtest.New(t)
c := srv.Client()

c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
    Name: aws.String("prod/db"), SecretString: aws.String("one"),
})
c.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
    SecretId: aws.String("prod/db"), SecretString: aws.String("two"),
})

cur, _ := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String("prod/db")})
// *cur.SecretString == "two"
old, _ := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
    SecretId: aws.String("prod/db"), VersionStage: aws.String("AWSPREVIOUS"),
})
// *old.SecretString == "one"
```

## Supported operations

| Operation | Notes |
|---|---|
| `CreateSecret` | string or binary; description, tags, `ClientRequestToken`. Duplicate name → `ResourceExistsException` |
| `PutSecretValue` | adds a version; `VersionStages` honoured |
| `UpdateSecret` | description; a new value adds a version |
| `GetSecretValue` | by name or ARN; by `VersionId` or `VersionStage` (default `AWSCURRENT`) |
| `DescribeSecret` | including `VersionIdsToStages` |
| `ListSecrets` | all live secrets, sorted by name; no filters, no pagination |
| `DeleteSecret` | recovery window (secret becomes unreadable) or `ForceDeleteWithoutRecovery` |
| `RestoreSecret` | undoes a scheduled deletion |

Staging labels follow AWS: the newest version is `AWSCURRENT`, the one before it `AWSPREVIOUS`.
Errors are the real typed errors (`ResourceNotFoundException`, `InvalidRequestException`, ...), so
`errors.As` works.

## Not supported

Everything else, e.g. `RotateSecret`, `TagResource`, `BatchGetSecretValue`, `ListSecretVersionIds`,
resource policies, replication. They fail with `UnknownOperationException: minism: awssm <Op> not
implemented`.

## Test helpers (not part of the AWS API)

- `srv.Value(name) (string, bool)`: current string value, read straight from the server.

## Notes

- Region is `us-east-1` and credentials are fixed fake ones; retries are disabled so errors surface at once.
- ARNs look like `arn:aws:secretsmanager:us-east-1:123456789012:secret:<name>-<6 digits>`.
