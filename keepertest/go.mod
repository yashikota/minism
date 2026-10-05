module github.com/yashikota/minism/keepertest

go 1.25.0

replace github.com/yashikota/minism => ../

require (
	github.com/keeper-security/secrets-manager-go/core v1.7.0
	github.com/yashikota/minism v0.0.0
)
