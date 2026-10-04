package azsecretstest_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"

	"github.com/yashikota/minism/azsecretstest"
)

func TestLifecycle(t *testing.T) {
	ctx := context.Background()
	srv := azsecretstest.New(t)
	c := srv.Client()

	// dashes in the name exercise the path-parameter parsing of the fake transport
	const name = "my-db-password"
	for _, v := range []string{"one", "two"} {
		if _, err := c.SetSecret(ctx, name, azsecrets.SetSecretParameters{Value: to.Ptr(v)}, nil); err != nil {
			t.Fatal(err)
		}
	}

	got, err := c.GetSecret(ctx, name, "", nil)
	if err != nil || *got.Value != "two" {
		t.Fatalf("latest = %v, %v", got.Value, err)
	}
	old, err := c.GetSecret(ctx, name, versionOf(t, c, name, 0), nil)
	if err != nil || *old.Value != "one" {
		t.Fatalf("oldest = %v, %v", old.Value, err)
	}

	if _, err := c.UpdateSecretProperties(ctx, name, "", azsecrets.UpdateSecretPropertiesParameters{
		Tags: map[string]*string{"env": to.Ptr("test")},
	}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ = c.GetSecret(ctx, name, "", nil)
	if *got.Tags["env"] != "test" {
		t.Fatalf("tags = %v", got.Tags)
	}

	var names []string
	for p := c.NewListSecretPropertiesPager(nil); p.More(); {
		page, err := p.NextPage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range page.Value {
			names = append(names, s.ID.Name())
		}
	}
	if len(names) != 1 || names[0] != name {
		t.Fatalf("list = %v", names)
	}

	if _, err := c.DeleteSecret(ctx, name, nil); err != nil {
		t.Fatal(err)
	}
	_, err = c.GetSecret(ctx, name, "", nil)
	var re *azcore.ResponseError
	if !errors.As(err, &re) || re.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %v", err)
	}
	if srv.Len() != 0 {
		t.Fatalf("len = %d", srv.Len())
	}
}

func versionOf(t *testing.T, c *azsecrets.Client, name string, idx int) string {
	t.Helper()
	var vs []string
	for p := c.NewListSecretPropertiesVersionsPager(name, nil); p.More(); {
		page, err := p.NextPage(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range page.Value {
			vs = append(vs, s.ID.Version())
		}
	}
	return vs[idx]
}
