package keepertest_test

import (
	"strings"
	"testing"

	"github.com/keeper-security/secrets-manager-go/core"

	"github.com/yashikota/minism/keepertest"
)

func login(title, user, pass string) *core.RecordCreate {
	rc := core.NewRecordCreate("login", title)
	rc.Fields = []interface{}{
		map[string]interface{}{"type": "login", "value": []interface{}{user}},
		map[string]interface{}{"type": "password", "value": []interface{}{pass}},
	}
	return rc
}

func TestRecordLifecycle(t *testing.T) {
	srv := keepertest.New(t)
	sm := srv.Client()

	uid, err := sm.CreateSecretWithRecordData("", srv.FolderUID(), login("db", "admin", "one"))
	if err != nil {
		t.Fatal(err)
	}
	if srv.Len() != 1 {
		t.Fatalf("len = %d", srv.Len())
	}

	recs, err := sm.GetSecrets([]string{uid})
	if err != nil || len(recs) != 1 {
		t.Fatalf("get = %v, %v", recs, err)
	}
	rec := recs[0]
	if rec.Title() != "db" || rec.GetFieldValueByType("password") != "one" {
		t.Fatalf("record = %q / %q", rec.Title(), rec.GetFieldValueByType("password"))
	}

	rec.SetPassword("two")
	if err := sm.Save(rec); err != nil {
		t.Fatal(err)
	}
	if err := sm.Save(rec); err == nil { // same revision again: stale
		t.Fatal("stale revision must be rejected")
	}
	got, err := sm.GetSecretByTitle("db")
	if err != nil || got.GetFieldValueByType("password") != "two" || got.Revision != 2 {
		t.Fatalf("after save = %v rev %v, %v", got.GetFieldValueByType("password"), got.Revision, err)
	}
	if v, err := sm.GetNotation("keeper://" + uid + "/field/password"); err != nil || len(v) != 1 || v[0] != "two" {
		t.Fatalf("notation = %v, %v", v, err)
	}

	st, err := sm.DeleteSecrets([]string{uid, "nope"})
	if err != nil || st[uid] != "ok" || !strings.Contains(st["nope"], "access_denied") {
		t.Fatalf("delete = %v, %v", st, err)
	}
	if recs, _ := sm.GetSecrets(nil); len(recs) != 0 {
		t.Fatalf("records left: %d", len(recs))
	}
}

func TestSeedAndFolders(t *testing.T) {
	srv := keepertest.New(t)
	uid := srv.Seed(login("seeded", "u", "p"))
	sm := srv.Client()

	recs, err := sm.GetSecrets(nil)
	if err != nil || len(recs) != 1 || recs[0].Uid != uid {
		t.Fatalf("get = %v, %v", recs, err)
	}
	folders, err := sm.GetFolders()
	if err != nil || len(folders) != 1 || folders[0].FolderUid != srv.FolderUID() || folders[0].Name != "folder" {
		t.Fatalf("folders = %+v, %v", folders, err)
	}
}
