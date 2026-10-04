package openbaotest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/openbao/openbao/api/v2"

	"github.com/yashikota/minism/openbaotest"
)

func TestKVv2Lifecycle(t *testing.T) {
	ctx := context.Background()
	kv := openbaotest.New(t).Client().KVv2("secret")

	for _, pw := range []string{"one", "two"} {
		if _, err := kv.Put(ctx, "app/db", map[string]any{"pw": pw}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := kv.Get(ctx, "app/db")
	if err != nil {
		t.Fatal(err)
	}
	if got.Data["pw"] != "two" || got.VersionMetadata.Version != 2 {
		t.Fatalf("latest = %v v%d", got.Data, got.VersionMetadata.Version)
	}
	old, err := kv.GetVersion(ctx, "app/db", 1)
	if err != nil || old.Data["pw"] != "one" {
		t.Fatalf("v1 = %v, %v", old, err)
	}
	if err := kv.DeleteMetadata(ctx, "app/db"); err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Get(ctx, "app/db"); !errors.Is(err, api.ErrSecretNotFound) {
		t.Fatalf("want ErrSecretNotFound, got %v", err)
	}
}
