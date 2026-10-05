# keepertest

In-memory **Keeper Secrets Manager** for tests. `Client()` returns a real `*core.SecretsManager`
(Keeper's Go SDK) that is already bound to an in-memory application with **one shared folder**.

Keeper encrypts everything, so the server does real crypto with the SDK's own helpers. It can do
that because the SDK has a test-only hook: `NewSecretsManager` accepts a `**core.Context` whose
`Transport` replaces the network, and the SDK stores each request's transmission key in it.

## Install

```
go get github.com/yashikota/minism/keepertest
```

## Usage

```go
srv := keepertest.New(t)
sm := srv.Client()

rc := core.NewRecordCreate("login", "db")
rc.Fields = []interface{}{
    map[string]interface{}{"type": "login", "value": []interface{}{"admin"}},
    map[string]interface{}{"type": "password", "value": []interface{}{"one"}},
}
uid, _ := sm.CreateSecretWithRecordData("", srv.FolderUID(), rc)

recs, _ := sm.GetSecrets([]string{uid})
// recs[0].GetFieldValueByType("password") == "one"

recs[0].SetPassword("two")
sm.Save(recs[0])
```

## Supported

| SDK call | Notes |
|---|---|
| `GetSecrets`, `GetSecretByTitle`, notation (`keeper://...`) | optional UID filter |
| `GetFolders` | the one shared folder, named `folder` |
| `CreateSecretWithRecordData` | **must** use `srv.FolderUID()` as the folder |
| `Save` | rejects a stale revision (as Keeper does) and bumps the revision |
| `DeleteSecrets` | per-record status `ok`, or `access_denied` for unknown UIDs |
| `CompleteTransaction` | accepted, no-op |

## Not supported

File attachments and upload, folder create / update / delete, records shared directly to the app
(everything lives in the shared folder), links, real transaction semantics. They answer 501 or are
no-ops as noted above.

## Test helpers (not part of the Keeper API)

- `srv.FolderUID() string`: the folder to create records in.
- `srv.Seed(*core.RecordCreate) string`: preload a record, returns its UID.
- `srv.RecordJSON(uid) (string, bool)`: decrypted record data.
- `srv.Len() int`: number of records.

## Notes

- Each `Client()` call returns a new `SecretsManager` with its own fresh in-memory configuration; all of them see the same records.
- Creating a record works through the folder key. The copy of the record key wrapped for the app owner's public key cannot be read here and is not needed.
