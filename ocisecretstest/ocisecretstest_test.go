package ocisecretstest_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/secrets"
	"github.com/oracle/oci-go-sdk/v65/vault"

	"github.com/yashikota/minism/ocisecretstest"
)

func b64(s string) *string { return common.String(base64.StdEncoding.EncodeToString([]byte(s))) }

func TestLifecycle(t *testing.T) {
	ctx := context.Background()
	srv := ocisecretstest.New(t)
	env := srv.Env()

	created, err := env.Vault.CreateSecret(ctx, vault.CreateSecretRequest{CreateSecretDetails: vault.CreateSecretDetails{
		CompartmentId: &env.CompartmentID, SecretName: common.String("db-pass"), VaultId: &env.VaultID, KeyId: &env.KeyID,
		SecretContent: vault.Base64SecretContentDetails{Content: b64("one")},
	}})
	if err != nil || *created.CurrentVersionNumber != 1 || created.LifecycleState != vault.SecretLifecycleStateActive {
		t.Fatalf("create = %+v, %v", created.Secret, err)
	}
	if _, err := env.Vault.CreateSecret(ctx, vault.CreateSecretRequest{CreateSecretDetails: vault.CreateSecretDetails{
		CompartmentId: &env.CompartmentID, SecretName: common.String("db-pass"), VaultId: &env.VaultID, KeyId: &env.KeyID,
		SecretContent: vault.Base64SecretContentDetails{Content: b64("x")},
	}}); err == nil {
		t.Fatal("duplicate name must fail")
	} else if se, ok := common.IsServiceError(err); !ok || se.GetHTTPStatusCode() != http.StatusConflict {
		t.Fatalf("want 409, got %v", err)
	}

	if _, err := env.Vault.UpdateSecret(ctx, vault.UpdateSecretRequest{SecretId: created.Id, UpdateSecretDetails: vault.UpdateSecretDetails{
		SecretContent: vault.Base64SecretContentDetails{Content: b64("two")},
	}}); err != nil {
		t.Fatal(err)
	}

	cur, err := env.Secrets.GetSecretBundle(ctx, secrets.GetSecretBundleRequest{SecretId: created.Id})
	if err != nil {
		t.Fatal(err)
	}
	content := cur.SecretBundleContent.(secrets.Base64SecretBundleContentDetails)
	if dec, _ := base64.StdEncoding.DecodeString(*content.Content); string(dec) != "two" || *cur.VersionNumber != 2 {
		t.Fatalf("current = %q v%d", dec, *cur.VersionNumber)
	}
	prev, err := env.Secrets.GetSecretBundle(ctx, secrets.GetSecretBundleRequest{SecretId: created.Id, Stage: secrets.GetSecretBundleStagePrevious})
	if err != nil {
		t.Fatal(err)
	}
	if dec, _ := base64.StdEncoding.DecodeString(*prev.SecretBundleContent.(secrets.Base64SecretBundleContentDetails).Content); string(dec) != "one" {
		t.Fatalf("previous = %q", dec)
	}
	byName, err := env.Secrets.GetSecretBundleByName(ctx, secrets.GetSecretBundleByNameRequest{SecretName: common.String("db-pass"), VaultId: &env.VaultID})
	if err != nil || *byName.VersionNumber != 2 {
		t.Fatalf("byName = %+v, %v", byName.SecretBundle, err)
	}

	list, err := env.Vault.ListSecrets(ctx, vault.ListSecretsRequest{CompartmentId: &env.CompartmentID})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("list = %+v, %v", list.Items, err)
	}
	vs, err := env.Vault.ListSecretVersions(ctx, vault.ListSecretVersionsRequest{SecretId: created.Id})
	if err != nil || len(vs.Items) != 2 {
		t.Fatalf("versions = %+v, %v", vs.Items, err)
	}

	if _, err := env.Vault.ScheduleSecretDeletion(ctx, vault.ScheduleSecretDeletionRequest{SecretId: created.Id}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Secrets.GetSecretBundle(ctx, secrets.GetSecretBundleRequest{SecretId: created.Id}); err == nil {
		t.Fatal("bundle of a secret pending deletion must not be readable")
	}
	if _, err := env.Vault.CancelSecretDeletion(ctx, vault.CancelSecretDeletionRequest{SecretId: created.Id}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Secrets.GetSecretBundle(ctx, secrets.GetSecretBundleRequest{SecretId: created.Id}); err != nil {
		t.Fatalf("after cancel: %v", err)
	}
	if v, ok := srv.Content("db-pass"); !ok || v != *content.Content {
		t.Fatalf("Content = %q %v", v, ok)
	}
}
