package onepasswordtest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/1password/onepassword-sdk-go"

	"github.com/yashikota/minism/onepasswordtest"
)

func TestItemsAndResolve(t *testing.T) {
	ctx := context.Background()
	srv := onepasswordtest.New(t)
	c := srv.Client()
	vid := srv.AddVault("Prod")

	created, err := c.Items().Create(ctx, onepassword.ItemCreateParams{
		VaultID: vid, Title: "db", Category: onepassword.ItemCategoryLogin,
		Fields: []onepassword.ItemField{
			{ID: "username", Title: "username", FieldType: onepassword.ItemFieldTypeText, Value: "admin"},
			{ID: "password", Title: "password", FieldType: onepassword.ItemFieldTypeConcealed, Value: "one"},
		},
	})
	if err != nil || created.Version != 1 {
		t.Fatalf("create = %+v, %v", created, err)
	}

	got, err := c.Secrets().Resolve(ctx, "op://Prod/db/password")
	if err != nil || got != "one" {
		t.Fatalf("resolve = %q, %v", got, err)
	}

	got2, _ := c.Items().Get(ctx, vid, created.ID)
	got2.Fields[1].Value = "two"
	updated, err := c.Items().Put(ctx, got2)
	if err != nil || updated.Version != 2 {
		t.Fatalf("put = %+v, %v", updated, err)
	}
	if _, err := c.Items().Put(ctx, got2); err == nil { // stale version
		t.Fatal("stale Put must fail")
	}
	if v, _ := c.Secrets().Resolve(ctx, "op://Prod/db/password"); v != "two" {
		t.Fatalf("after put = %q", v)
	}

	all, err := c.Secrets().ResolveAll(ctx, []string{"op://Prod/db/username", "op://Prod/db/nope", "op://Nope/db/password"})
	if err != nil {
		t.Fatal(err)
	}
	if r := all.IndividualResponses["op://Prod/db/username"]; r.Content == nil || r.Content.Secret != "admin" {
		t.Fatalf("username = %+v", r)
	}
	if r := all.IndividualResponses["op://Prod/db/nope"]; r.Error == nil || r.Error.Type != onepassword.ResolveReferenceErrorTypeVariantFieldNotFound {
		t.Fatalf("nope = %+v", r)
	}
	if r := all.IndividualResponses["op://Nope/db/password"]; r.Error == nil || r.Error.Type != onepassword.ResolveReferenceErrorTypeVariantVaultNotFound {
		t.Fatalf("vault = %+v", r)
	}

	list, _ := c.Items().List(ctx, vid)
	if len(list) != 1 {
		t.Fatalf("list = %v", list)
	}
	if err := c.Items().Archive(ctx, vid, created.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := c.Items().List(ctx, vid); len(list) != 0 {
		t.Fatalf("archived item listed: %v", list)
	}
	if err := c.Items().Delete(ctx, vid, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Items().Get(ctx, vid, created.ID); !errors.Is(err, onepasswordtest.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := c.Items().Files().Read(ctx, vid, "x", onepassword.FileAttributes{}); err == nil {
		t.Fatal("unimplemented ops must error")
	}
}

func TestVaults(t *testing.T) {
	ctx := context.Background()
	c := onepasswordtest.New(t).Client()
	desc := "d"
	v, err := c.Vaults().Create(ctx, onepassword.VaultCreateParams{Title: "A", Description: &desc})
	if err != nil {
		t.Fatal(err)
	}
	ov, err := c.Vaults().List(ctx)
	if err != nil || len(ov) != 1 || ov[0].ID != v.ID {
		t.Fatalf("list = %v, %v", ov, err)
	}
	if err := c.Vaults().Delete(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Vaults().GetOverview(ctx, v.ID); !errors.Is(err, onepasswordtest.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
