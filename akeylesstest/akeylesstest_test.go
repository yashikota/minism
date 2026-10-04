package akeylesstest_test

import (
	"context"
	"testing"

	"github.com/akeylesslabs/akeyless-go/v5"

	"github.com/yashikota/minism/akeylesstest"
)

func TestStaticSecretLifecycle(t *testing.T) {
	ctx := context.Background()
	srv := akeylesstest.New(t)
	c := srv.Client()

	auth, _, err := c.Auth(ctx).Body(akeyless.Auth{AccessId: akeyless.PtrString("p-1")}).Execute()
	if err != nil || auth.GetToken() == "" {
		t.Fatalf("auth = %+v, %v", auth, err)
	}
	token := auth.GetToken()

	out, _, err := c.CreateSecret(ctx).Body(akeyless.CreateSecret{Name: "/app/db", Value: "one", Token: &token}).Execute()
	if err != nil || out.GetName() != "/app/db" {
		t.Fatalf("create = %+v, %v", out, err)
	}
	if _, _, err := c.CreateSecret(ctx).Body(akeyless.CreateSecret{Name: "/app/db", Value: "x", Token: &token}).Execute(); err == nil {
		t.Fatal("duplicate create must fail")
	}

	if _, _, err := c.UpdateSecretVal(ctx).Body(akeyless.UpdateSecretVal{Name: "/app/db", Value: "two", Token: &token}).Execute(); err != nil {
		t.Fatal(err)
	}
	got, _, err := c.GetSecretValue(ctx).Body(akeyless.GetSecretValue{Names: []string{"/app/db"}, Token: &token}).Execute()
	if err != nil || got["/app/db"] != "two" {
		t.Fatalf("latest = %v, %v", got, err)
	}
	old, _, err := c.GetSecretValue(ctx).Body(akeyless.GetSecretValue{Names: []string{"/app/db"}, Version: akeyless.PtrInt32(1), Token: &token}).Execute()
	if err != nil || old["/app/db"] != "one" {
		t.Fatalf("v1 = %v, %v", old, err)
	}

	d, _, err := c.DescribeItem(ctx).Body(akeyless.DescribeItem{Name: "/app/db", Token: &token}).Execute()
	if err != nil || d.GetLastVersion() != 2 || len(d.ItemVersions) != 2 {
		t.Fatalf("describe = %+v, %v", d, err)
	}
	l, _, err := c.ListItems(ctx).Body(akeyless.ListItems{Path: akeyless.PtrString("/app"), Token: &token}).Execute()
	if err != nil || len(l.Items) != 1 {
		t.Fatalf("list = %+v, %v", l, err)
	}

	if _, _, err := c.DeleteItem(ctx).Body(akeyless.DeleteItem{Name: "/app/db", Token: &token}).Execute(); err != nil {
		t.Fatal(err)
	}
	if _, resp, err := c.GetSecretValue(ctx).Body(akeyless.GetSecretValue{Names: []string{"/app/db"}, Token: &token}).Execute(); err == nil || resp.StatusCode != 404 {
		t.Fatalf("want 404: %v %v", resp, err)
	}
}
