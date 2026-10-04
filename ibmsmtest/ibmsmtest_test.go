package ibmsmtest_test

import (
	"net/http"
	"testing"

	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/secrets-manager-go-sdk/v2/secretsmanagerv2"

	"github.com/yashikota/minism/ibmsmtest"
)

func TestArbitraryLifecycle(t *testing.T) {
	srv := ibmsmtest.New(t)
	c := srv.Client()

	res, _, err := c.CreateSecret(&secretsmanagerv2.CreateSecretOptions{SecretPrototype: &secretsmanagerv2.ArbitrarySecretPrototype{
		Name: core.StringPtr("db-pass"), SecretType: core.StringPtr("arbitrary"),
		Payload: core.StringPtr("one"), Labels: []string{"env:test"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	created := res.(*secretsmanagerv2.ArbitrarySecret)
	if *created.Name != "db-pass" || *created.SecretGroupID != "default" || *created.Payload != "one" {
		t.Fatalf("created = %+v", created)
	}

	_, resp, err := c.CreateSecret(&secretsmanagerv2.CreateSecretOptions{SecretPrototype: &secretsmanagerv2.ArbitrarySecretPrototype{
		Name: core.StringPtr("db-pass"), SecretType: core.StringPtr("arbitrary"), Payload: core.StringPtr("x"),
	}})
	if err == nil || resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate: %v %v", resp, err)
	}

	if _, _, err := c.CreateSecretVersion(&secretsmanagerv2.CreateSecretVersionOptions{
		SecretID: created.ID, SecretVersionPrototype: &secretsmanagerv2.ArbitrarySecretVersionPrototype{Payload: core.StringPtr("two")},
	}); err != nil {
		t.Fatal(err)
	}
	got, _, err := c.GetSecret(&secretsmanagerv2.GetSecretOptions{ID: created.ID})
	if err != nil || *got.(*secretsmanagerv2.ArbitrarySecret).Payload != "two" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	prev, _, err := c.GetSecretVersion(&secretsmanagerv2.GetSecretVersionOptions{SecretID: created.ID, ID: core.StringPtr("previous")})
	if err != nil || *prev.(*secretsmanagerv2.ArbitrarySecretVersion).Payload != "one" {
		t.Fatalf("previous = %+v, %v", prev, err)
	}
	byName, _, err := c.GetSecretByNameType(&secretsmanagerv2.GetSecretByNameTypeOptions{
		SecretType: core.StringPtr("arbitrary"), Name: core.StringPtr("db-pass"), SecretGroupName: core.StringPtr("default"),
	})
	if err != nil || *byName.(*secretsmanagerv2.ArbitrarySecret).ID != *created.ID {
		t.Fatalf("byName = %+v, %v", byName, err)
	}

	if _, _, err := c.UpdateSecretMetadata(&secretsmanagerv2.UpdateSecretMetadataOptions{
		ID: created.ID, SecretMetadataPatch: map[string]interface{}{"description": "patched"},
	}); err != nil {
		t.Fatal(err)
	}
	list, _, err := c.ListSecrets(&secretsmanagerv2.ListSecretsOptions{})
	if err != nil || *list.TotalCount != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	vs, _, err := c.ListSecretVersions(&secretsmanagerv2.ListSecretVersionsOptions{SecretID: created.ID})
	if err != nil || len(vs.Versions) != 2 {
		t.Fatalf("versions = %+v, %v", vs, err)
	}

	if _, err := c.DeleteSecret(&secretsmanagerv2.DeleteSecretOptions{ID: created.ID}); err != nil {
		t.Fatal(err)
	}
	if _, resp, err := c.GetSecret(&secretsmanagerv2.GetSecretOptions{ID: created.ID}); err == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404: %v %v", resp, err)
	}
}
