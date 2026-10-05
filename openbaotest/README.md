# openbaotest

**OpenBao** KV for tests. `Client()` returns a real `*api.Client`
(`github.com/openbao/openbao/api/v2`) talking to an in-memory server that speaks the OpenBao HTTP
API for `sys/mounts` and the KV secrets engine (v1 and v2).

This is **not OpenBao**. It is a small reimplementation of that part of the wire protocol; there
is no auth, policy, lease, seal or any other engine. OpenBao and Vault share this protocol, so the
server is the same one that backs [vaulttest](../vaulttest/README.md) (`internal/kvfake`).

## Install

```
go get github.com/yashikota/minism/openbaotest
```

## Usage

```go
c := openbaotest.New(t).Client()
kv := c.KVv2("secret")                       // kv-v2 is mounted at secret/, like `bao server -dev`

kv.Put(ctx, "app/db", map[string]any{"pw": "one"})
kv.Put(ctx, "app/db", map[string]any{"pw": "two"})

latest, _ := kv.Get(ctx, "app/db")           // latest.Data["pw"] == "two", Version == 2
v1, _ := kv.GetVersion(ctx, "app/db", 1)     // v1.Data["pw"] == "one"
```

## Supported

- **`sys/mounts`**: mount and unmount engines of type `kv` / `kv-v2`, list mounts. A mount with
  `Options: {"version": "2"}` is KV v2; otherwise KV v1. Other engine types are rejected (400).
- **KV v2** (`secret/` by default): write (with `cas` check-and-set), read latest or a given
  version, soft delete the latest version, `delete` / `undelete` / `destroy` specific versions,
  metadata read (versions, timestamps), metadata delete (removes all versions), list.
- **KV v1**: read, write, delete, list.

Missing secrets return 404, so `KVv2.Get` returns `api.ErrSecretNotFound`.

## Not supported

Auth methods, policies, tokens, leases, transit, PKI, database and every other engine, namespaces,
`sys/*` other than mounts. Unimplemented paths answer 501.

## Test helpers

- `srv.Token() string`: the token the client is configured with. Any token is accepted.

## Notes

- Address is `http://openbao.invalid` and nothing is ever dialled.
- Retries are disabled so errors surface at once.
