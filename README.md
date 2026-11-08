# plamo-trans（Go版）

[日本語](README.md) | [English](README.en.md)

> [!IMPORTANT]
> **PLaMoモデルには利用条件があります。非商用利用は事前登録なしで無償ですが、商用利用には条件があります。**
> 商用利用（社内業務での利用を含む）は、利用者または所属会社の年間収入・売上が10億円以下で、PFNへの事前登録を行う場合に無償です。条件を満たさない商用利用には、PFNとの別途商用ライセンス契約が必要です。
> 利用前に [PLaMo Community License](https://plamo.preferredai.jp/info/plamo-community-license-en) と [PFNの公式説明](https://www.preferred.jp/en/blog/tech/plamo-3-translate-weight-release) を確認してください。

[PLaMo 3 Translate GGUF](https://huggingface.co/collections/pfnet/plamo-3-translate) を使う翻訳CLIです。0.8B（既定）・2B・8B・31Bを選択できます。Python版 `plamo-trans-nano-cli` の機能をGoに移植しています。

## ビルド

Go 1.23以降と、`llama serve` または `llama-server` が必要です。Goの依存ライブラリは初回ビルド時に取得されます。実行時にはPythonは不要です。

```bash
go build -o plamo-trans .
./plamo-trans --help
```

Git情報を取得できない作業環境でビルドが失敗する場合は、`go build -buildvcs=false -o plamo-trans .` を使ってください。

`llama` / `llama-server` をPATHから探して自動起動します。`--llama-bin PATH` または `PLAMO_LLAMA_BIN` でも指定できます。起動したサーバーは終了時に停止します。Linux/macOSでは子プロセスもまとめて停止します。Windowsではサーバーのプロセスを停止します。既存のサーバーを使う場合は `--server URL` を指定してください。

## 使い方

```bash
./plamo-trans "会議は来週の月曜日に変更されました。"
echo "The bank was muddy." | ./plamo-trans -t ja --form polite --context "雨上がりの川辺"
./plamo-trans -f doc.md -o doc.en.md -t en --glossary "管理画面=>dashboard"
./plamo-trans --server http://127.0.0.1:8080 -t en "こんにちは"
./plamo-trans --model /path/to/model.gguf -t en "こんにちは"
./plamo-trans -i -t en
./plamo-trans --size 2b --quant Q4_K_M -t en "こんにちは"
./plamo-trans --size 8b -f doc.md -o doc.en.md -t en
```

| オプション | 動作 |
| --- | --- |
| `-t`, `--target LANG` | 翻訳先。省略または `auto` で日⇄英を自動判別 |
| `-s`, `--source LANG` | 翻訳元。明示的な翻訳先が必要 |
| `--form polite\|plain` | 日本語の文体。`-t ja` が必要 |
| `--glossary FILE\|'a=>b'` | 用語集。繰り返し指定可能 |
| `--context TEXT\|FILE` | 参考文脈。実在するファイルなら内容を使用 |
| `-f`, `--file PATH` | 文書を分割してストリーミング翻訳 |
| `-o`, `--output PATH` | `-f` の出力先。省略時は標準出力 |
| `-i`, `--interactive` | 対話モード |
| `--size SIZE` | `0.8b`（既定）、`2b`、`8b`、`31b` |
| `--quant NAME` | `Q4_K_M`（既定）、`Q3_K_M`、`Q2_K`。0.8B・2Bは `Q4_0` にも対応 |
| `--model PATH` | ローカルGGUFを直接使用 |
| `--offline` | モデルのダウンロードを禁止 |
| `--server URL` | 既存サーバーを使用 |
| `--llama-bin PATH` | llamaの実行ファイル |
| `--ctx N` | サーバーのコンテキストサイズ。既定16384 |
| `--max-tokens N` | 出力トークン上限。既定2048 |
| `-v`, `--verbose` | サーバーログを標準エラーへ表示 |

引数のテキストは空白で連結します。オプションはテキストの後にも指定できます。`--` 以降はすべて本文として扱います。本文を指定しない場合は標準入力を読みます。複数行の標準入力も文書として分割翻訳します。

用語集ファイルはUTF-8で、1行に1つの `翻訳元=>翻訳先` を記述します。空行と `#` で始まる行は無視します。

文書は約2000文字ずつに分割します。Markdownのバッククォート3個によるコードフェンスは翻訳せず保持します。段落間の空白と末尾改行を保持し、長い段落は文や空白の境界を優先して分割します。長い段落の翻訳片同士は空白でつなぎます。翻訳済みの直近4組、合計3000文字までを次の翻訳の履歴として使います。

## 対話モード

**空行で入力を確定**します。複数行を貼り付けた場合も、空行までを1つの翻訳入力として扱います。

```text
> 今日はいい天気です。

The weather is nice today.
```

`/target LANG` で翻訳先を変更し履歴をクリア、`/clear` で履歴をクリア、`/quit` または `/exit` で終了します。コマンドは入力の先頭行に書くと即実行されます。Ctrl-Dも入力を確定します（空の入力では終了）。

端末では矢印キー、Home/Endによる編集と入力履歴を利用できます。履歴は `$XDG_STATE_HOME/plamo-trans/history`、未設定時は `~/.local/state/plamo-trans/history` に最大1000行保存します。プロンプトと編集表示は標準エラーへ出すので、標準出力には翻訳結果だけが流れます。パイプ入力にも対応しています。

## モデルとオフライン動作

`--size` と `--quant` で公式GGUFを選択します。サイズ指定は大文字・小文字を区別しません（例: `2b` と `2B` は同じ）。指定しなければ従来どおり0.8Bの `Q4_K_M` を使用します。

| `--size` | 対応量子化 |
| --- | --- |
| `0.8b`、`2b` | `Q4_K_M`、`Q4_0`、`Q3_K_M`、`Q2_K` |
| `8b`、`31b` | `Q4_K_M`、`Q3_K_M`、`Q2_K` |

組み合わせに対応するファイルが公式配布にない場合は、ダウンロードせず引数エラーで終了します。モデルの配布先とファイル名はサイズごとに切り替わり、キャッシュも分かれます。`--model PATH` または `--server URL` を指定した場合は、それらを優先して使用し、`--size` / `--quant` によるモデル選択は行いません。

`HF_HUB_CACHE`、または `$HF_HOME/hub`（既定 `~/.cache/huggingface/hub`）の `models--pfnet--plamo-3-translate-<size>-GGUF/refs/main`（`<size>` は `0.8b`、`2b`、`8b`、`31b`） が指すsnapshotを優先して使います。Python版や `hf download` のキャッシュを共有できます。キャッシュがなければHugging Faceから取得し、同じsnapshot構造に保存します。ダウンロードは一時ファイル経由で行い、未完了のモデルをキャッシュとして扱いません。

```bash
./plamo-trans --offline "text"
HF_HUB_OFFLINE=1 ./plamo-trans "text"
```

オフライン時はモデル取得の通信を行いません。キャッシュがなければエラーです。自動起動するllamaにも `--offline` と `HF_HUB_OFFLINE=1` / `TRANSFORMERS_OFFLINE=1` を渡します。`--model` と `--server` はモデルのキャッシュ探索・取得を行いません。`--server` を指定した場合、翻訳リクエストは指定URLへ送信します。

オンライン取得では `HF_ENDPOINT`、`HF_TOKEN`、`HUGGING_FACE_HUB_TOKEN`、`HF_TOKEN_PATH`（既定 `$HF_HOME/token`）に対応しています。

## 入力と出力

入力はUTF-8です。通常の1リクエストは30000文字、標準入力とファイルは200万文字までです。文脈は8000文字、用語集の各語は500文字までです。本文・文脈・用語に予約トークン `<|plamo:` を含むとエラーになります（文書翻訳でそのまま保持するコードフェンス内と、予約トークンを記したインラインコードを除く）。端末に表示する翻訳からは制御文字を除去します。出力ファイルとパイプにはそのまま出力します。

終了コードは成功 `0`、処理エラー `1`、引数エラー `2`、シグナルによる中断 `130` です。

Python版からの改善として、文書末尾の改行を正確に保持し、無効な `/target` の指定時には変更前の設定を維持します。

## 検証

```bash
go test -race ./...
go vet ./...
```

模擬HTTPサーバーを使い、APIの送信内容とエラー、ストリーミング、CLI、文書保持、REPL、モデル取得・キャッシュ、サーバーの起動・終了を検証します。テスト時に実モデルの取得・推論は行いません。

## モデルのライセンス

モデルの利用条件は [PLaMo Community License](https://plamo.preferredai.jp/info/plamo-community-license-en) を確認してください。
