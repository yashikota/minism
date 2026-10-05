# onepasswordtest

In-memory **1Password** for tests. `Client()` returns a real `*onepassword.Client` whose
`ItemsAPI`, `VaultsAPI` and `SecretsAPI` fields (which the SDK exports) are replaced by in-memory
implementations. The WASM core and the network are never involved.

## Install

```
go get github.com/yashikota/minism/onepasswordtest
```

Not tagged yet, so `go get` will not resolve for now: see [Adding it to your project today](../README.md#adding-it-to-your-project-today).

## Usage

```go
srv := onepasswordtest.New(t)
c := srv.Client()
vaultID := srv.AddVault("Prod")

item, _ := c.Items().Create(ctx, onepassword.ItemCreateParams{
    VaultID: vaultID, Title: "db", Category: onepassword.ItemCategoryLogin,
    Fields: []onepassword.ItemField{
        {ID: "password", Title: "password", FieldType: onepassword.ItemFieldTypeConcealed, Value: "one"},
    },
})

pw, _ := c.Secrets().Resolve(ctx, "op://Prod/db/password") // "one"
```

## Supported

| API | Operations |
|---|---|
| `Items()` | `Create`, `CreateAll`, `Get`, `GetAll`, `Put` (rejects a stale `Version`), `Delete`, `DeleteAll`, `Archive`, `List` |
| `Vaults()` | `Create`, `List`, `GetOverview`, `Get`, `Update`, `Delete` |
| `Secrets()` | `Resolve`, `ResolveAll` for `op://<vault>/<item>/[<section>/]<field>` |

- Vault and item can be referenced by **title or ID**; fields by title or ID.
- `List` returns active items only (archived ones are hidden); filter arguments are ignored.
- `ResolveAll` reports per-reference errors with the SDK's own types (`FieldNotFound`,
  `VaultNotFound`, `ItemNotFound`, `Parsing`).
- Single-call errors wrap `onepasswordtest.ErrNotFound` (use `errors.Is`).

## Not supported

- `Items().Shares()` and `Items().Files()` return an "not implemented" error.
- Group permission methods on `Vaults()` return an error.
- **`Environments()` and `Groups()` are `nil`**: calling methods on them panics.
- `?attribute=` suffixes on secret references are ignored.

## Test helpers (not part of the 1Password API)

- `srv.AddVault(title) string`: creates a vault and returns its ID.

## Notes

- IDs are 26-digit numbers, valid in 1Password's ID alphabet.
