# cfsecretstest

In-memory **Cloudflare Secrets Store** for tests. `Client()` returns a real `*cloudflare.Client`
(`cloudflare-go/v6`) whose HTTP client is an in-memory server.

## Install

```
go get github.com/yashikota/minism/cfsecretstest
```

Not tagged yet, so `go get` will not resolve for now: see [Adding it to your project today](../README.md#adding-it-to-your-project-today).

## Usage

```go
srv := cfsecretstest.New(t)
c := srv.Client()
acct := cloudflare.F(cfsecretstest.AccountID)

res, _ := c.SecretsStore.Stores.Secrets.New(ctx, srv.StoreID(), secrets_store.StoreSecretNewParams{
    AccountID: acct,
    Body: []secrets_store.StoreSecretNewParamsBody{{
        Name: cloudflare.F("DB_PASSWORD"), Value: cloudflare.F("one"), Scopes: cloudflare.F([]string{"workers"}),
    }},
})

v, _ := srv.Value(srv.StoreID(), "DB_PASSWORD") // "one": see "Secret values" below
_ = res
```

## Supported

| Resource | Operations |
|---|---|
| Stores | `New`, `List`, `Delete` |
| Secrets | `New` (bulk, takes a list), `List` (`page`, `per_page`), `Get`, `Edit` (value, scopes, comment), `Delete`, `Duplicate` |

A store named `default` already exists; `srv.StoreID()` returns its ID. A duplicate secret name in a
store → 409. Errors are real `*cloudflare.Error` values with the status code.

## Not supported

`BulkDelete`, quotas, and the rest of the Cloudflare API. They answer 501.

## Secret values

As in the real API, **a secret's value cannot be read back through the client**; responses carry
metadata only. To assert what your code stored, use the test helper below.

## Test helpers (not part of the Cloudflare API)

- `cfsecretstest.AccountID`: the only account ID accepted; any other account gets 403.
- `srv.StoreID() string`: ID of the default store.
- `srv.Value(storeID, name) (string, bool)`: the stored value.

## Notes

- Retries are disabled so errors surface at once.
