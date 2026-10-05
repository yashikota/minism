# vaulttest

Vault-compatible KV server in memory for tests. The official `*api.Client` talks to it
through a `RoundTripper`; no listener, no port.

```go
c := vaulttest.New(t).Client()
c.KVv2("secret").Put(ctx, "app/db", map[string]any{"pw": "x"})
```

Supported: `sys/mounts` (mount/unmount/list for `kv`), KV v2 (data, metadata, delete,
undelete, destroy, list, check-and-set, versions) and KV v1. Not a Vault: no auth, policies,
leases or other engines. Anything else answers 501.

The protocol implementation is shared with its sibling provider in `internal/kvfake`.
