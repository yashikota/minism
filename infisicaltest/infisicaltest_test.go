package infisicaltest_test

import (
	"errors"
	"net/http"
	"testing"

	infisical "github.com/infisical/go-sdk"

	"github.com/yashikota/minism/infisicaltest"
)

func TestSecretLifecycle(t *testing.T) {
	srv := infisicaltest.New(t)
	s := srv.Client().Secrets()
	const proj, env = "proj", "dev"

	created, err := s.Create(infisical.CreateSecretOptions{ProjectID: proj, Environment: env, SecretKey: "DB", SecretValue: "one"})
	if err != nil || created.Version != 1 || created.Type != "shared" || created.SecretPath != "/" {
		t.Fatalf("create = %+v, %v", created, err)
	}
	var apiErr *infisical.APIError
	if _, err := s.Create(infisical.CreateSecretOptions{ProjectID: proj, Environment: env, SecretKey: "DB"}); !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("duplicate = %v", err)
	}

	if _, err := s.Update(infisical.UpdateSecretOptions{ProjectID: proj, Environment: env, SecretKey: "DB", NewSecretValue: "two"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Retrieve(infisical.RetrieveSecretOptions{ProjectID: proj, Environment: env, SecretKey: "DB"})
	if err != nil || got.SecretValue != "two" || got.Version != 2 {
		t.Fatalf("latest = %+v, %v", got, err)
	}
	old, err := s.Retrieve(infisical.RetrieveSecretOptions{ProjectSlug: proj, Environment: env, SecretKey: "DB", Version: 1})
	if err != nil || old.SecretValue != "one" {
		t.Fatalf("v1 = %+v, %v", old, err)
	}

	if _, err := s.Create(infisical.CreateSecretOptions{ProjectID: proj, Environment: env, SecretPath: "/app/db", SecretKey: "X", SecretValue: "x"}); err != nil {
		t.Fatal(err)
	}
	root, _ := s.List(infisical.ListSecretsOptions{ProjectID: proj, Environment: env})
	rec, _ := s.List(infisical.ListSecretsOptions{ProjectID: proj, Environment: env, Recursive: true})
	if len(root) != 1 || len(rec) != 2 {
		t.Fatalf("root=%d recursive=%d", len(root), len(rec))
	}

	if _, err := s.Delete(infisical.DeleteSecretOptions{ProjectID: proj, Environment: env, SecretKey: "DB"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Retrieve(infisical.RetrieveSecretOptions{ProjectID: proj, Environment: env, SecretKey: "DB"}); !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404, got %v", err)
	}
}

func TestFoldersAndAuth(t *testing.T) {
	c := infisicaltest.New(t).Client()
	f, err := c.Folders().Create(infisical.CreateFolderOptions{ProjectID: "p", Environment: "dev", Name: "app"})
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := c.Folders().List(infisical.ListFoldersOptions{ProjectID: "p", Environment: "dev"}); len(l) != 1 {
		t.Fatalf("list = %v", l)
	}
	if _, err := c.Folders().Update(infisical.UpdateFolderOptions{FolderID: f.ID, ProjectID: "p", Environment: "dev", NewName: "svc"}); err != nil {
		t.Fatal(err)
	}
	if l, _ := c.Folders().List(infisical.ListFoldersOptions{ProjectID: "p", Environment: "dev"}); len(l) != 1 || l[0].Name != "svc" {
		t.Fatalf("renamed = %v", l)
	}
	if _, err := c.Folders().Delete(infisical.DeleteFolderOptions{FolderName: "svc", ProjectID: "p", Environment: "dev"}); err != nil {
		t.Fatal(err)
	}

	cred, err := c.Auth().UniversalAuthLogin("id", "secret")
	if err != nil || cred.AccessToken == "" || c.Auth().GetAccessToken() != cred.AccessToken {
		t.Fatalf("login = %+v, %v", cred, err)
	}
}
