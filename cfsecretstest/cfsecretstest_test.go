package cfsecretstest_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/cloudflare/cloudflare-go/v6"
	"github.com/cloudflare/cloudflare-go/v6/secrets_store"

	"github.com/yashikota/minism/cfsecretstest"
)

func TestSecretLifecycle(t *testing.T) {
	ctx := context.Background()
	srv := cfsecretstest.New(t)
	c := srv.Client()
	store := srv.StoreID()

	created, err := c.SecretsStore.Stores.Secrets.New(ctx, store, secrets_store.StoreSecretNewParams{
		AccountID: cloudflare.F(cfsecretstest.AccountID),
		Body: []secrets_store.StoreSecretNewParamsBody{{
			Name: cloudflare.F("DB_PASSWORD"), Value: cloudflare.F("one"),
			Scopes: cloudflare.F([]string{"workers"}), Comment: cloudflare.F("c"),
		}},
	})
	if err != nil || len(created.Result) != 1 || created.Result[0].Name != "DB_PASSWORD" {
		t.Fatalf("create = %+v, %v", created, err)
	}
	id := created.Result[0].ID

	if _, err := c.SecretsStore.Stores.Secrets.New(ctx, store, secrets_store.StoreSecretNewParams{
		AccountID: cloudflare.F(cfsecretstest.AccountID),
		Body: []secrets_store.StoreSecretNewParamsBody{{
			Name: cloudflare.F("DB_PASSWORD"), Value: cloudflare.F("x"), Scopes: cloudflare.F([]string{"workers"}),
		}},
	}); err == nil {
		t.Fatal("duplicate name must fail")
	}

	if _, err := c.SecretsStore.Stores.Secrets.Edit(ctx, store, id, secrets_store.StoreSecretEditParams{
		AccountID: cloudflare.F(cfsecretstest.AccountID), Value: cloudflare.F("two"),
	}); err != nil {
		t.Fatal(err)
	}
	if v, ok := srv.Value(store, "DB_PASSWORD"); !ok || v != "two" {
		t.Fatalf("Value = %q %v", v, ok)
	}

	got, err := c.SecretsStore.Stores.Secrets.Get(ctx, store, id, secrets_store.StoreSecretGetParams{AccountID: cloudflare.F(cfsecretstest.AccountID)})
	if err != nil || got.Comment != "c" || got.Status != "active" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	list, err := c.SecretsStore.Stores.Secrets.List(ctx, store, secrets_store.StoreSecretListParams{AccountID: cloudflare.F(cfsecretstest.AccountID)})
	if err != nil || len(list.Result) != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}

	if _, err := c.SecretsStore.Stores.Secrets.Delete(ctx, store, id, secrets_store.StoreSecretDeleteParams{AccountID: cloudflare.F(cfsecretstest.AccountID)}); err != nil {
		t.Fatal(err)
	}
	_, err = c.SecretsStore.Stores.Secrets.Get(ctx, store, id, secrets_store.StoreSecretGetParams{AccountID: cloudflare.F(cfsecretstest.AccountID)})
	var apiErr *cloudflare.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %v", err)
	}
}

func TestStores(t *testing.T) {
	ctx := context.Background()
	c := cfsecretstest.New(t).Client()
	st, err := c.SecretsStore.Stores.New(ctx, secrets_store.StoreNewParams{AccountID: cloudflare.F(cfsecretstest.AccountID), Name: cloudflare.F("extra")})
	if err != nil || st.Name != "extra" {
		t.Fatalf("store = %+v, %v", st, err)
	}
	l, err := c.SecretsStore.Stores.List(ctx, secrets_store.StoreListParams{AccountID: cloudflare.F(cfsecretstest.AccountID)})
	if err != nil || len(l.Result) != 2 {
		t.Fatalf("stores = %+v, %v", l, err)
	}
}
