# vaulttest

Real HashiCorp Vault core running in memory for tests. No fake, no listener, no port:
the official `*api.Client` talks to Vault's real HTTP handler through a `RoundTripper`.

```go
c := vaulttest.New(t).Client()
c.KVv2("secret").Put(ctx, "app/db", map[string]any{"pw": "x"})
```

## go.mod requirements (read this)

Vault's core is not meant to be imported: its `go.mod` uses `replace` for its own `sdk`
and `api`, and `replace` does not propagate to dependents. Your `go.mod` must repeat them:

```
replace github.com/hashicorp/vault/sdk => github.com/hashicorp/vault/sdk v0.19.1-0.20260305014005-ffe7023c481d
replace github.com/hashicorp/vault/api => github.com/hashicorp/vault/api v1.21.1-0.20260305014005-ffe7023c481d
replace github.com/tencentcloud/tencentcloud-sdk-go => github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common v1.0.479
```

(The pseudo-versions are the in-tree `sdk`/`api` of the Vault v1.21.4 commit; the third line
removes an "ambiguous import" between tencentcloud's monolith and its split modules.)

## License

Vault is BUSL-1.1. Test-only use is generally fine, but check it applies to you.
