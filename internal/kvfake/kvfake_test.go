package kvfake_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/yashikota/minism/internal/kvfake"
	"github.com/yashikota/minism/internal/rtfake"
)

func do(t *testing.T, c *http.Client, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, "http://vault.invalid/v1/"+path, strings.NewReader(body))
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return res.StatusCode, out
}

func TestKV2VersionsAndDeletion(t *testing.T) {
	c := rtfake.Client(kvfake.New())
	for _, v := range []string{"one", "two"} {
		if st, _ := do(t, c, "POST", "secret/data/app/db", `{"data":{"pw":"`+v+`"}}`); st != 200 {
			t.Fatalf("put = %d", st)
		}
	}
	st, out := do(t, c, "GET", "secret/data/app/db", "")
	if st != 200 || out["data"].(map[string]any)["data"].(map[string]any)["pw"] != "two" {
		t.Fatalf("latest = %d %v", st, out)
	}
	if _, out := do(t, c, "GET", "secret/data/app/db?version=1", ""); out["data"].(map[string]any)["data"].(map[string]any)["pw"] != "one" {
		t.Fatalf("v1 = %v", out)
	}
	if st, _ := do(t, c, "POST", "secret/data/app/db", `{"data":{"pw":"x"},"options":{"cas":1}}`); st != 400 {
		t.Fatalf("stale cas = %d", st)
	}

	do(t, c, "DELETE", "secret/data/app/db", "")
	if st, out := do(t, c, "GET", "secret/data/app/db", ""); st != 404 || out["data"] == nil {
		t.Fatalf("soft-deleted = %d %v", st, out)
	}
	do(t, c, "POST", "secret/undelete/app/db", `{"versions":[2]}`)
	if st, _ := do(t, c, "GET", "secret/data/app/db", ""); st != 200 {
		t.Fatalf("undeleted = %d", st)
	}
	do(t, c, "POST", "secret/destroy/app/db", `{"versions":[1]}`)
	if st, _ := do(t, c, "GET", "secret/data/app/db?version=1", ""); st != 404 {
		t.Fatalf("destroyed = %d", st)
	}
	do(t, c, "DELETE", "secret/metadata/app/db", "")
	if st, _ := do(t, c, "GET", "secret/metadata/app/db", ""); st != 404 {
		t.Fatalf("metadata delete = %d", st)
	}
}

func TestListMountsAndV1(t *testing.T) {
	c := rtfake.Client(kvfake.New())
	do(t, c, "POST", "secret/data/a/b", `{"data":{"k":"v"}}`)
	do(t, c, "POST", "secret/data/top", `{"data":{"k":"v"}}`)
	_, out := do(t, c, "GET", "secret/metadata/?list=true", "")
	keys := out["data"].(map[string]any)["keys"].([]any)
	if len(keys) != 2 || keys[0] != "a/" || keys[1] != "top" {
		t.Fatalf("keys = %v", keys)
	}

	if st, _ := do(t, c, "POST", "sys/mounts/legacy", `{"type":"kv"}`); st != 204 {
		t.Fatalf("mount = %d", st)
	}
	if st, _ := do(t, c, "POST", "sys/mounts/legacy", `{"type":"kv"}`); st != 400 {
		t.Fatalf("remount = %d", st)
	}
	do(t, c, "POST", "legacy/x", `{"k":"v"}`)
	if st, out := do(t, c, "GET", "legacy/x", ""); st != 200 || out["data"].(map[string]any)["k"] != "v" {
		t.Fatalf("v1 get = %d %v", st, out)
	}
	if st, _ := do(t, c, "POST", "sys/mounts/db", `{"type":"database"}`); st != 400 {
		t.Fatalf("unsupported type = %d", st)
	}
}
