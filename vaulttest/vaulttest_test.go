package vaulttest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/vault/api"

	"github.com/yashikota/minism/vaulttest"
)

func TestKVv2Lifecycle(t *testing.T) {
	ctx := context.Background()
	kv := vaulttest.New(t).Client().KVv2("secret")

	if _, err := kv.Put(ctx, "app/db", map[string]any{"pw": "one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Put(ctx, "app/db", map[string]any{"pw": "two"}); err != nil {
		t.Fatal(err)
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

func TestServersAreIsolated(t *testing.T) {
	ctx := context.Background()
	a, b := vaulttest.New(t), vaulttest.New(t)
	if _, err := a.Client().KVv2("secret").Put(ctx, "x", map[string]any{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Client().KVv2("secret").Get(ctx, "x"); !errors.Is(err, api.ErrSecretNotFound) {
		t.Fatalf("state leaked between servers: %v", err)
	}
}
