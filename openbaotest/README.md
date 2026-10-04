# openbaotest

Real OpenBao core (physical/inmem) + real KV engine, in memory, for tests.
The official `*api.Client` talks to OpenBao's real HTTP handler via a `RoundTripper`.

```go
c := openbaotest.New(t).Client()
c.KVv2("secret").Put(ctx, "app/db", map[string]any{"pw": "x"})
```

## How it reaches `internal/`

OpenBao's repository is `package main` plus `internal/`, so there is no public package to
import and `//go:linkname` has nothing to link against. Go's `internal` rule only compares
import paths, so `./shim` is a tiny module declared as `github.com/openbao/openbao/v2/shim`:
that makes it a legal importer of `internal/vault`, `internal/http` and `internal/builtin/logical/kv`.
No linkname, no `unsafe`, no struct mirroring.

## go.mod requirements

`replace` does not propagate, so your `go.mod` needs:

```
require github.com/openbao/openbao/v2/shim v0.0.0
replace github.com/openbao/openbao/v2/shim => github.com/yashikota/minism/openbaotest/shim <tag>
replace github.com/openbao/openbao/sdk/v2 => github.com/openbao/openbao/sdk/v2 v2.7.1
```

OpenBao v2.7.1 needs Go >= 1.27 (the toolchain is fetched automatically).
