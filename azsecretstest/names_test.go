package azsecretstest_test

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"

	"github.com/yashikota/minism/azsecretstest"
)

// Regression: azsecrets/fake v1.5.0 mis-parses "/secrets/{name}/{version}".
func TestNamesAndVersionsRoundTrip(t *testing.T) {
	ctx := context.Background()
	c := azsecretstest.New(t).Client()
	for _, name := range []string{"plain", "my-db", "a1-b2-c3"} {
		set, err := c.SetSecret(ctx, name, azsecrets.SetSecretParameters{Value: to.Ptr("v-" + name)}, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, ver := range []string{"", set.ID.Version()} {
			got, err := c.GetSecret(ctx, name, ver, nil)
			if err != nil || *got.Value != "v-"+name {
				t.Errorf("%s@%q: %v, %v", name, ver, got.Value, err)
			}
		}
	}
}
