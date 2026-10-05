# minism

[English](README.md) | 日本語

Go のテスト用の偽の secret manager です。メモリ上で動きます。ネットワークも Docker もアカウントも要りません。

## はじめる(2 分)

1. 使う Provider をインストールします(例は AWS。他は [下の一覧](#provider-を選ぶ)):

   ```
   go get github.com/yashikota/minism/awssmtest
   ```

2. テストの中で、偽物を作って client を受け取ります:

   ```go
   client := awssmtest.New(t).Client() // 普通の *secretsmanager.Client。向き先だけが偽物

   client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
       Name: aws.String("prod/db"), SecretString: aws.String("s3cret"),
   })
   out, _ := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String("prod/db")})
   // *out.SecretString == "s3cret"
   ```

3. `go test` を実行します。

自分のコードは本物の AWS client を使い続けます。偽物なのは、その向こう側のサーバーだけです。

## Provider を選ぶ

1 行が 1 つの Go module です。パッケージ名は path の最後の部分です。

**大手クラウド**

| Provider | インストール | 返ってくる client |
|---|---|---|
| AWS Secrets Manager | `go get github.com/yashikota/minism/awssmtest` | `*secretsmanager.Client` |
| Google Secret Manager | `go get github.com/yashikota/minism/gcpsmtest` | `*secretmanager.Client` |
| Azure Key Vault (secrets) | `go get github.com/yashikota/minism/azsecretstest` | `*azsecrets.Client` |
| OCI Vault | `go get github.com/yashikota/minism/ocisecretstest` | `VaultsClient` + `SecretsClient` |
| IBM Cloud Secrets Manager | `go get github.com/yashikota/minism/ibmsmtest` | `*SecretsManagerV2` |

**Vault 系とセルフホスト**

| Provider | インストール | 返ってくる client |
|---|---|---|
| HashiCorp Vault | `go get github.com/yashikota/minism/vaulttest` | `*api.Client` |
| OpenBao | `go get github.com/yashikota/minism/openbaotest` | `*api.Client` |
| Infisical | `go get github.com/yashikota/minism/infisicaltest` | `infisical.InfisicalClientInterface` |

**パスワード管理・secret の SaaS**

| Provider | インストール | 返ってくる client |
|---|---|---|
| 1Password | `go get github.com/yashikota/minism/onepasswordtest` | `*onepassword.Client` |
| Keeper Secrets Manager | `go get github.com/yashikota/minism/keepertest` | `*core.SecretsManager` |
| Akeyless | `go get github.com/yashikota/minism/akeylesstest` | `*akeyless.V2ApiService` |
| Cloudflare Secrets Store | `go get github.com/yashikota/minism/cfsecretstest` | `*cloudflare.Client` |

各 Provider のコードの隣に README があります(例: [awssmtest](awssmtest/README.md))。対応している操作、
対応していない操作、動く例が書いてあります。README は英語のみです。

## 知っておくこと

- **未対応の呼び出しは、必ずエラーになります。** `minism: <provider> <operation> not implemented` が返り、黙って成功することはありません。
- **認証は確認しません。** どんな認証情報やトークンでも通ります。
- **`New(t)` のたびに新しいサーバーができます。** テスト間で何も共有されず、テストが終わると破棄されます。
- **`Value(...)` と `Seed(...)` はテスト用のヘルパーです。** Provider の API ではありません。準備と検証にだけ使ってください。
- **本物のサービスではありません。** 作成・取得・更新・削除・一覧・バージョンを扱います。IAM、ローテーション、レプリケーション、KMS はありません。

## 呼び出しが失敗したら

1. メッセージに `not implemented` が含まれる: その操作は未対応です。その Provider の README の「Supported」を見てください。
2. それ以外: 偽物のサーバーに対して、本物の client が本物のエラー(見つからない、すでに存在する、など)を報告しています。テストの準備を確認してください。

<details>
<summary>完全な例: アプリのコードとそのテスト(5 分)</summary>

自分のコードを小さな interface に依存させておき、テストでは minism の client を渡します。

`app.go`(minism に関するものは何も無い):

```go
package myapp

import (
    "context"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// このアプリが使う AWS client の一部だけ。*secretsmanager.Client はこれを満たす。
type SecretsAPI interface {
    GetSecretValue(ctx context.Context, in *secretsmanager.GetSecretValueInput,
        opts ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

func LoadDBPassword(ctx context.Context, c SecretsAPI) (string, error) {
    out, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String("prod/db")})
    if err != nil {
        return "", err
    }
    return aws.ToString(out.SecretString), nil
}
```

`app_test.go`(コードが期待する状態を作ってから、コードを実行する):

```go
package myapp_test

import (
    "context"
    "testing"

    "github.com/aws/aws-sdk-go-v2/aws"
    "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
    "github.com/yashikota/minism/awssmtest"

    myapp "example.com/myapp"
)

func TestLoadDBPassword(t *testing.T) {
    ctx := context.Background()
    client := awssmtest.New(t).Client()

    _, err := client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
        Name: aws.String("prod/db"), SecretString: aws.String("s3cret"),
    })
    if err != nil {
        t.Fatal(err)
    }

    got, err := myapp.LoadDBPassword(ctx, client)
    if err != nil || got != "s3cret" {
        t.Fatalf("got %q, %v", got, err)
    }
}

func TestMissingSecret(t *testing.T) {
    // 空のサーバー: 本物の client が本物の ResourceNotFoundException を返す。
    if _, err := myapp.LoadDBPassword(context.Background(), awssmtest.New(t).Client()); err == nil {
        t.Fatal("expected an error")
    }
}
```

</details>

<details>
<summary>仕組み</summary>

```
自分のコード ──► 本物の SDK client ──► minism の transport ──► メモリ上のサーバー
```

transport は各 SDK が用意しているものによって決まります。差し替えられる `http.Client`(AWS、IBM、OCI、
Akeyless、Cloudflare、Vault、OpenBao)、gRPC 接続(GCP)、ベンダー提供の fake(Azure)、公開されている
API フィールド(1Password)、公開されている interface(Infisical)、テスト専用の transport の口(Keeper)です。
ソケットは開きません。CI は、外への経路が無い network namespace の中で全 module を動かします。

</details>

<details>
<summary>開発</summary>

共通のコードは root module の `internal/` にあります。`memstore`(バージョン付きのメモリ上ストア)、
`rtfake`(`http.Handler` を `RoundTripper` にする)、`kvfake`(`vaulttest` と `openbaotest` が共有する
Vault 互換の KV プロトコル)です。

`go.work` でローカルでは全 module をまとめて扱います。CI は各 module を単体で(`GOWORK=off`)ビルドして vet し、
ネットワークを切った状態(`unshare --net`、`GOPROXY=off`)でテストします。

```
for d in $(find . -name go.mod -exec dirname {} \;); do (cd $d && GOWORK=off go test -race ./...); done
```

バージョンはタグです。root module は `v0.1.0`、Provider ごとに `awssmtest/v0.1.0` のようになります。
`internal/` を変えたら、先に root にタグを打ち、そのあと各 module の `require` を更新します。

上流の不具合を 1 つ回避しています。`azsecrets/fake` v1.5.0 は `/secrets/{name}/{version}` を
正しく解析できません(詳しくは [azsecretstest](azsecretstest/README.md))。

</details>
