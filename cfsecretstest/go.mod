module github.com/yashikota/minism/cfsecretstest

go 1.25.0

replace github.com/yashikota/minism => ../

require (
	github.com/cloudflare/cloudflare-go/v6 v6.10.0
	github.com/yashikota/minism v0.0.0
)

require (
	github.com/tidwall/gjson v1.14.4 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
)
