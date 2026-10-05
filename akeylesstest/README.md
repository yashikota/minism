# akeylesstest

In-memory **Akeyless** gateway for tests. `Client()` returns a real `*akeyless.V2ApiService`
(`akeyless-go/v5`) whose HTTP client is an in-memory server.

## Install

```
go get github.com/yashikota/minism/akeylesstest
```

Not tagged yet, so `go get` will not resolve for now: see [Adding it to your project today](../README.md#adding-it-to-your-project-today).

## Usage

```go
srv := akeylesstest.New(t)
c := srv.Client()

auth, _, _ := c.Auth(ctx).Body(akeyless.Auth{AccessId: akeyless.PtrString("p-1")}).Execute()
tok := auth.GetToken()

c.CreateSecret(ctx).Body(akeyless.CreateSecret{Name: "/app/db", Value: "one", Token: &tok}).Execute()
c.UpdateSecretVal(ctx).Body(akeyless.UpdateSecretVal{Name: "/app/db", Value: "two", Token: &tok}).Execute()

got, _, _ := c.GetSecretValue(ctx).Body(akeyless.GetSecretValue{Names: []string{"/app/db"}, Token: &tok}).Execute()
// got["/app/db"] == "two"
```

## Supported

Only **static secrets**.

| Operation | Notes |
|---|---|
| `Auth` | any credentials succeed and return a token |
| `CreateSecret` | duplicate name → 400 |
| `GetSecretValue` | one or more `Names`; optional `Version` |
| `UpdateSecretVal` | adds a version; with `keep-prev-version=false` it overwrites the latest |
| `DeleteItem` | |
| `ListItems` | items under a `Path`; one page |
| `DescribeItem` | last version, version list, timestamps |

Missing items answer 404 with Akeyless's `{"error": "..."}` body.

## Not supported

Dynamic and rotated secrets, certificates, keys, auth methods, roles, targets, pagination.
Everything else answers 501 `minism: akeyless <path> not implemented`.

## Test helpers (not part of the Akeyless API)

- `srv.Value(name) (string, bool)`: latest value.

## Notes

- The token is not validated, but pass one anyway: real code does, and it is part of the request body.
- Names are normalised to start with `/`.
