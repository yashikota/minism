module github.com/yashikota/minism/azsecretstest

go 1.25.0

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.21.0
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets v1.5.0
	github.com/yashikota/minism v0.0.0
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/internal v1.2.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/text v0.35.0 // indirect
)

replace github.com/yashikota/minism => ../
