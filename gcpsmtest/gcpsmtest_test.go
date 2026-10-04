package gcpsmtest_test

import (
	"context"
	"testing"

	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/yashikota/minism/gcpsmtest"
)

func code(err error) codes.Code { return status.Code(err) }

func TestLifecycle(t *testing.T) {
	ctx := context.Background()
	srv := gcpsmtest.New(t)
	c := srv.Client()
	const parent, id = "projects/p", "db-pass"
	name := parent + "/secrets/" + id

	if _, err := c.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{Parent: parent, SecretId: id}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{Parent: parent, SecretId: id}); code(err) != codes.AlreadyExists {
		t.Fatalf("want AlreadyExists, got %v", err)
	}
	for _, v := range []string{"one", "two"} {
		if _, err := c.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{Parent: name, Payload: &secretmanagerpb.SecretPayload{Data: []byte(v)}}); err != nil {
			t.Fatal(err)
		}
	}

	latest, err := c.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{Name: name + "/versions/latest"})
	if err != nil || string(latest.Payload.Data) != "two" || latest.Name != name+"/versions/2" {
		t.Fatalf("latest = %v, %v", latest, err)
	}
	v1, err := c.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{Name: name + "/versions/1"})
	if err != nil || string(v1.Payload.Data) != "one" {
		t.Fatalf("v1 = %v, %v", v1, err)
	}

	if _, err := c.DisableSecretVersion(ctx, &secretmanagerpb.DisableSecretVersionRequest{Name: name + "/versions/2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{Name: name + "/versions/latest"}); code(err) != codes.FailedPrecondition {
		t.Fatalf("want FailedPrecondition, got %v", err)
	}
	if got, ok := srv.Value(name); !ok || string(got) != "one" {
		t.Fatalf("Value = %q %v", got, ok)
	}

	n := 0
	it := c.ListSecretVersions(ctx, &secretmanagerpb.ListSecretVersionsRequest{Parent: name})
	for _, err := it.Next(); err != iterator.Done; _, err = it.Next() {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 2 {
		t.Fatalf("versions = %d", n)
	}

	if err := c.DeleteSecret(ctx, &secretmanagerpb.DeleteSecretRequest{Name: name}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{Name: name}); code(err) != codes.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
}
