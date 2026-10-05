# minism

[English](README.md) | 日本語

Go のテスト用の偽の secret manager です。AWS Secrets Manager、Google Secret Manager、
Azure Key Vault、HashiCorp Vault、1Password などに対応しています。テストのプロセスの中で動くので、
インターネットも Docker も本物のアカウントも要りません。

## 困りごと

アプリが AWS Secrets Manager から DB のパスワードを読むとします。どうテストしますか。

- 本物の AWS を呼ぶ: 遅く、認証情報が要り、テスト同士が干渉することもある。
- 自分で AWS client の mock を書く: 量が多く、しかも「mock した呼び出しをしているか」しか確かめられない。
  本物の client で動くかどうかは分からない。

## minism がやること

メモリ上に偽の Secrets Manager を用意します。そこに秘密を入れておくと、アプリは本番と同じように、
普通の AWS client ライブラリでそれを読めます。

```go
fake := awssmtest.New(t)       // 空の、メモリ上の AWS Secrets Manager
client := fake.Client()        // 普通の *secretsmanager.Client。向き先だけが偽物

client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{   // 秘密を入れて...
    Name: aws.String("prod/db"), SecretString: aws.String("s3cret"),
})
password := LoadDBPassword(ctx, client)                        // ...自分のコードが読み出す
```

client は AWS SDK の本物なので、リクエストの組み立て、署名、エラーの型、リトライといった、
自分のコード側の処理は実際に動きます。偽物なのは client の向こう側のサーバーだけです。
`New(t)` のたびに空の新しい偽物ができ、テストが終わると消えます。

以下の他の Provider でも考え方は同じで、その Provider 自身の client がそのまま返ってきます。
動く完全な例は [使い方](#使い方) を見てください。

## Provider 一覧

| Provider | インストール | 返ってくるもの | 裏側 | README(英語) |
|---|---|---|---|---|
| AWS Secrets Manager | `go get github.com/yashikota/minism/awssmtest` | `*secretsmanager.Client` | 自前のメモリ上の endpoint | [awssmtest](awssmtest/README.md) |
| Google Secret Manager | `go get github.com/yashikota/minism/gcpsmtest` | `*secretmanager.Client` | 自前のメモリ上の gRPC サーバー | [gcpsmtest](gcpsmtest/README.md) |
| Azure Key Vault (secrets) | `go get github.com/yashikota/minism/azsecretstest` | `*azsecrets.Client` | Microsoft 公式の `azsecrets/fake` | [azsecretstest](azsecretstest/README.md) |
| HashiCorp Vault | `go get github.com/yashikota/minism/vaulttest` | `*api.Client` | 自前のメモリ上の KV サーバー | [vaulttest](vaulttest/README.md) |
| OpenBao | `go get github.com/yashikota/minism/openbaotest` | `*api.Client` | Vault と同じ KV サーバー | [openbaotest](openbaotest/README.md) |
| 1Password | `go get github.com/yashikota/minism/onepasswordtest` | `*onepassword.Client` | client にメモリ上の API を差し込む | [onepasswordtest](onepasswordtest/README.md) |
| Infisical | `go get github.com/yashikota/minism/infisicaltest` | `infisical.InfisicalClientInterface` | SDK の interface をメモリ上で実装 | [infisicaltest](infisicaltest/README.md) |
| OCI Vault | `go get github.com/yashikota/minism/ocisecretstest` | `VaultsClient` + `SecretsClient` | 自前のメモリ上の endpoint | [ocisecretstest](ocisecretstest/README.md) |
| IBM Cloud Secrets Manager | `go get github.com/yashikota/minism/ibmsmtest` | `*SecretsManagerV2` | 自前のメモリ上の endpoint | [ibmsmtest](ibmsmtest/README.md) |
| Akeyless | `go get github.com/yashikota/minism/akeylesstest` | `*akeyless.V2ApiService` | 自前のメモリ上の endpoint | [akeylesstest](akeylesstest/README.md) |
| Cloudflare Secrets Store | `go get github.com/yashikota/minism/cfsecretstest` | `*cloudflare.Client` | 自前のメモリ上の endpoint | [cfsecretstest](cfsecretstest/README.md) |
| Keeper Secrets Manager | `go get github.com/yashikota/minism/keepertest` | `*core.SecretsManager` | 自前のメモリ上の暗号化 endpoint | [keepertest](keepertest/README.md) |

Provider ごとに別の Go module なので、1 つ使っても、その Provider の SDK しか依存に入りません。
パッケージ名は path の最後の部分です(`awssmtest`、`gcpsmtest` など)。
各 Provider の README は今のところ英語のみです。

## 使い方

どの Provider でも流れは同じです。自分のコードは小さな interface(または SDK の client 型)に
依存させておき、テストでは minism の client を渡します。

**自分のコード**(`app.go`): minism に関するものは何も無い。

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

**テスト**(`app_test.go`): コードが期待する状態を作ってから、コードを実行する。

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
    client := awssmtest.New(t).Client() // 新しい、メモリ上の AWS Secrets Manager

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
    // 空のサーバー: 本物の client が、本物の ResourceNotFoundException を返す。
    if _, err := myapp.LoadDBPassword(context.Background(), awssmtest.New(t).Client()); err == nil {
        t.Fatal("expected an error")
    }
}
```

手順はこれだけです。`New(t)` で新しいサーバーを作り、`.Client()` で公式の client を受け取り、
必要な状態は SDK 自身を使って作ります。他の Provider も同じやり方で、それぞれの README に
同じ形の例があります。

### プロジェクトに入れる

```
go get github.com/yashikota/minism/awssmtest@latest
```

使いたい Provider ごとに `go get` を実行します(上の表に一覧があります)。各 Provider は
`awssmtest/v0.1.0` のようなタグで個別にバージョン管理されています。共通の root module は
`v0.1.0` で、自動的に一緒に取得されます。

## ここでの「偽物」の意味

これらは本物のサービスでは**ありませんし**、そうなることも目指していません。それぞれ、テストで
よく使う操作(作成・取得・更新・削除・一覧・バージョン)だけを実装しています。IAM、ローテーション、
レプリケーション、KMS、クォータなどはありません。

- **未対応の操作は、黙って成功せずにエラーになります。** `minism: <provider> <operation> not implemented`
  (または SDK 自身の「未実装」エラー)が返ります。これに当たったら、あなたのコードのバグではなく、
  minism 側に足りない機能があるということです。
- **認証は確認しません。** どんな認証情報やトークンでも通ります。
- **`New(t)` のたびに、新しい独立したサーバーができます。** テスト間で状態は共有されず、
  テストが終わると破棄されます。
- **フィクスチャ用ヘルパー**(`Value(...)` や `Seed(...)` など)で、テストからサーバーの状態を
  直接見たり、あらかじめ入れたりできます。これらは Provider の API ではありません。準備と
  検証にだけ使ってください。

何に対応していて何に対応していないかは、各 Provider の README に正確に書いてあります。

## 仕組み

```
自分のコード ──► 本物の SDK client ──► minism の transport ──► メモリ上のサーバー
                 (署名、リトライ、       (RoundTripper、          (Provider ごとの
                  エラーの型)             bufconn、または          小さな状態管理)
                                          SDK 自身のテスト用の口)
```

client をどう差し替えるかは、各 SDK が何を用意しているかで決まります。差し替えられる `http.Client`
(AWS、IBM、OCI、Akeyless、Cloudflare、Vault、OpenBao)、gRPC 接続(GCP)、ベンダー提供の fake(Azure)、
公開されている API フィールド(1Password)、公開されている interface(Infisical)、テスト専用の
transport の口(Keeper)、のいずれかです。どれもソケットは開きません。CI では、外への経路が無い
network namespace の中で全 module を動かして、これを確かめています。

## 開発

共通のコードは root module の `internal/` にあります。`memstore`(バージョン付きのメモリ上ストア)、
`rtfake`(`http.Handler` を `RoundTripper` にする)、`kvfake`(`vaulttest` と `openbaotest` が共有する、
Vault 互換の KV プロトコル)です。

`go.work` でローカルでは全 module をまとめて扱います。CI は各 module を単体で(`GOWORK=off`)ビルドし、
vet と依存の事前取得をしたあと、ネットワークを切った状態(`unshare --net`、`GOPROXY=off`)でテストします。

```
for d in $(find . -name go.mod -exec dirname {} \;); do (cd $d && GOWORK=off go test -race ./...); done
```

上流の既知の不具合を 1 つ回避しています。`azsecrets/fake` v1.5.0 は
`/secrets/{name}/{version}` を正しく解析できません。詳しくは [azsecretstest](azsecretstest/README.md)
を見てください。
