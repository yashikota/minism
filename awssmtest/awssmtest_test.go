package awssmtest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"

	"github.com/yashikota/minism/awssmtest"
)

func TestLifecycle(t *testing.T) {
	ctx := context.Background()
	srv := awssmtest.New(t)
	c := srv.Client()

	created, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name: aws.String("prod/db"), SecretString: aws.String("one"), Description: aws.String("d"),
		Tags: []types.Tag{{Key: aws.String("env"), Value: aws.String("test")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var exists *types.ResourceExistsException
	if _, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{Name: aws.String("prod/db")}); !errors.As(err, &exists) {
		t.Fatalf("want ResourceExistsException, got %v", err)
	}

	put, err := c.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{SecretId: aws.String("prod/db"), SecretString: aws.String("two")})
	if err != nil {
		t.Fatal(err)
	}

	cur, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: created.ARN})
	if err != nil || *cur.SecretString != "two" || *cur.VersionId != *put.VersionId {
		t.Fatalf("current = %v, %v", cur, err)
	}
	old, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String("prod/db"), VersionStage: aws.String("AWSPREVIOUS")})
	if err != nil || *old.SecretString != "one" || *old.VersionId != *created.VersionId {
		t.Fatalf("previous = %v, %v", old, err)
	}

	d, err := c.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String("prod/db")})
	if err != nil || *d.Description != "d" || len(d.VersionIdsToStages) != 2 || *d.Tags[0].Key != "env" {
		t.Fatalf("describe = %+v, %v", d, err)
	}
	l, err := c.ListSecrets(ctx, &secretsmanager.ListSecretsInput{})
	if err != nil || len(l.SecretList) != 1 {
		t.Fatalf("list = %v, %v", l, err)
	}

	if _, err := c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{SecretId: aws.String("prod/db")}); err != nil {
		t.Fatal(err)
	}
	var invalid *types.InvalidRequestException
	if _, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String("prod/db")}); !errors.As(err, &invalid) {
		t.Fatalf("want InvalidRequestException, got %v", err)
	}
	if _, err := c.RestoreSecret(ctx, &secretsmanager.RestoreSecretInput{SecretId: aws.String("prod/db")}); err != nil {
		t.Fatal(err)
	}
	if v, ok := srv.Value("prod/db"); !ok || v != "two" {
		t.Fatalf("after restore: %q %v", v, ok)
	}

	if _, err := c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{SecretId: aws.String("prod/db"), ForceDeleteWithoutRecovery: aws.Bool(true)}); err != nil {
		t.Fatal(err)
	}
	var nf *types.ResourceNotFoundException
	if _, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String("prod/db")}); !errors.As(err, &nf) {
		t.Fatalf("want ResourceNotFoundException, got %v", err)
	}
}

func TestBinaryAndUnimplemented(t *testing.T) {
	ctx := context.Background()
	c := awssmtest.New(t).Client()
	if _, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{Name: aws.String("bin"), SecretBinary: []byte{0, 1, 2}}); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String("bin")})
	if err != nil || string(got.SecretBinary) != "\x00\x01\x02" || got.SecretString != nil {
		t.Fatalf("binary = %v, %v", got, err)
	}
	if _, err := c.RotateSecret(ctx, &secretsmanager.RotateSecretInput{SecretId: aws.String("bin")}); err == nil {
		t.Fatal("RotateSecret must not silently succeed")
	}
}
