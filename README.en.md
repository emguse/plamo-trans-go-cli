# plamo-trans (Go)

[日本語](README.md) | [English](README.en.md)

> [!IMPORTANT]
> **The PLaMo model is subject to terms of use. Non-commercial use is free and requires no prior registration, but commercial use is subject to conditions.**
> Commercial use (including internal business use) is free if the user or their affiliated company's annual revenue or sales are 1 billion yen or less, and they complete prior registration with PFN. Commercial use that does not meet these conditions requires a separate commercial license agreement with PFN.
> Before use, please review the [PLaMo Community License](https://plamo.preferredai.jp/info/plamo-community-license-en) and [PFN's official explanation](https://www.preferred.jp/en/blog/tech/plamo-3-translate-weight-release).

This is a translation CLI that uses [PLaMo 3 Translate GGUF](https://huggingface.co/collections/pfnet/plamo-3-translate). You can choose from 0.8B (default), 2B, 8B, or 31B. It ports the functionality of the Python version `plamo-trans-nano-cli` to Go.

## Build

Requires Go 1.23 or later and `llama serve` or `llama-server`. Go dependencies are fetched during the initial build. No Python is required for runtime.

```bash
go build -o plamo-trans .
./plamo-trans --help
```

If the build fails in a work environment where Git information cannot be retrieved, use `go build -buildvcs=false -o plamo-trans .`.

Looks for `llama` / `llama-server` in the PATH and starts them automatically. You can also specify them using `--llama-bin PATH` or `PLAMO_LLAMA_BIN`. The started servers will stop upon termination. On Linux/macOS, child processes are also stopped. On Windows, cleanup stops the direct server process. To use an existing server, specify `--server URL`.

## Usage

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

| Option | Action |
| --- | --- |
| `-t`, `--target LANG` | Translation target. Omit or use `auto` to automatically detect Japanese ⇄ English |
| `-s`, `--source LANG` | Source. Requires an explicit translation target |
| `--form polite\|plain` | Japanese style. Requires `-t ja` |
| `--glossary FILE\|'a=>b'` | Glossary. Can be specified repeatedly |
| `--context TEXT\|FILE` | Reference context. Uses content from an existing file |
| `-f`, `--file PATH` | Split documents for streaming translation |
| `-o`, `--output PATH` | Output destination for `-f`. Standard output if omitted |
| `-i`, `--interactive` | Interactive mode |
| `--size SIZE` | `0.8b` (default), `2b`, `8b`, `31b` |
| `--quant NAME` | `Q4_K_M` (default), `Q3_K_M`, `Q2_K`. 0.8B and 2B also support `Q4_0` |
| `--model PATH` | Use a local GGUF directly |
| `--offline` | Prevent model downloads |
| `--server URL` | Use an existing server |
| `--llama-bin PATH` | llama executable |
| `--ctx N` | Server context size. Default 16384 |
| `--translate-code` | Preserve code fences and translate their contents |
| `--format text\|json` | Input format (default: `text`); `json` translates string values only |
| `--max-tokens N` | Output token limit. Default 2048 |
| `-v`, `--verbose` | Display server logs to standard error |

Arguments are concatenated using whitespace. Options can also be specified after the text. Everything following `--` is treated as the main body. If no main body is specified, standard input is read. Multiple lines of standard input are also split and translated as part of the document.

The glossary file is in UTF-8 format, with one `source=>destination` pair per line. Empty lines and lines starting with `#` are ignored.

Documents are split into chunks of approximately 2,000 characters. Markdown code fences consisting of three backticks are preserved without translation. Whitespace between paragraphs and trailing newlines are preserved; long paragraphs are split by prioritizing sentence and whitespace boundaries. Translation segments for long paragraphs are joined by whitespace. The four most recent translated pairs, totaling up to 3,000 characters, are used as the history for the next translation.


`--translate-code` sends fenced code contents to the model while preserving the fences. It supports files, stdin, positional text, and the REPL. The model may still return code unchanged. Reserved-token validation remains enabled; inline code containing reserved-token examples is still preserved.

`--format json` validates JSON and translates each nonblank string value, including values in nested objects and arrays. Keys, numbers, booleans, null, ordering, structure, and whitespace outside values are preserved. Markdown inside string values is treated as plain text; combining this with `--translate-code` is unnecessary. String escape spelling may change. Long values are split into approximately 2,000-character chunks, and translations are escaped as JSON. Files, stdin, positional text, and the REPL are supported. Values containing reserved tokens are rejected.

```bash
./plamo-trans --translate-code -f document.md -t en
./plamo-trans --format json -f messages.json -o messages.en.json -t en
```

## Interactive Mode

**Confirm input with an empty line**. When multiple lines are pasted, the entire sequence up to an empty line is treated as a single translation input.

```text
> 今日はいい天気です。

The weather is nice today.
```

Use `/target LANG` to change the target language and clear the history, `/clear` to clear the history, and `/quit` or `/exit` to exit. Commands entered on the first line of input are executed immediately. Ctrl-D also confirms the input (exiting on empty input).

In the terminal, use the arrow keys and Home/End to edit input and recall previous entries. Up to 1,000 lines are saved to `$XDG_STATE_HOME/plamo-trans/history`, or to `~/.local/state/plamo-trans/history` if unset. Prompts and edit displays are sent to standard error, so only the translation results are output to standard output. Pipe input is also supported.

## Models and Offline Behavior

Use `--size` and `--quant` to select the official GGUF. Model sizes are case-insensitive (for example, `2b` and `2B` are equivalent). If not specified, the 0.8B `Q4_K_M` is used as before.

| `--size` | Supported Quantization |
| --- | --- |
| `0.8b`, `2b` | `Q4_K_M`, `Q4_0`, `Q3_K_M`, `Q2_K` |
| `8b`, `31b` | `Q4_K_M`, `Q3_K_M`, `Q2_K` |

If a file corresponding to the combination is not available in the official distribution, the process will exit with an argument error instead of downloading it. The model distribution locations and filenames vary by size, and caching is also separate. If `--model PATH` or `--server URL` is specified, these are prioritized, and model selection via `--size` / `--quant` is not performed.

Priority is given to the snapshot pointed to by `models--pfnet--plamo-3-translate-<size>-GGUF/refs/main` (where `<size>` is `0.8b`, `2b`, `8b`, or `31b`) in `HF_HUB_CACHE` or `$HF_HOME/hub` (default: `~/.cache/huggingface/hub`). The Python version and `hf download` caches can be shared. If no cache exists, the file is retrieved from Hugging Face and saved in the same snapshot structure. Downloads are performed via temporary files, and incomplete models are not treated as caches.

```bash
./plamo-trans --offline "text"
HF_HUB_OFFLINE=1 ./plamo-trans "text"
```

When offline, no model retrieval communication is performed. If no cache exists, an error occurs. The CLI also passes `--offline` and sets `HF_HUB_OFFLINE=1` / `TRANSFORMERS_OFFLINE=1` for the automatically started llama server. `--model` and `--server` do not perform model cache lookup or retrieval. If `--server` is specified, translation requests are sent to the specified URL.

For online retrieval, `HF_ENDPOINT`, `HF_TOKEN`, `HUGGING_FACE_HUB_TOKEN`, and `HF_TOKEN_PATH` (default: `$HF_HOME/token`) are supported.

## Input and Output

Input must be UTF-8. A single request is limited to 30,000 characters; standard input and files are limited to 2 million characters. Context is limited to 8,000 characters, and each source or target term in the glossary to 500 characters. Reserved `<|plamo:` tokens are rejected in input text, context, and glossary terms. During document translation, fenced code and inline code containing reserved-token examples are kept verbatim and excluded from model requests. Control characters are removed from translations displayed on the terminal. Output files and pipes receive the translation unchanged.

The exit codes are `0` for success, `1` for processing error, `2` for argument error, and `130` for signal interruption.

The Go port preserves trailing newlines exactly and keeps the previous REPL settings when an invalid `/target` is specified.

## Verification

```bash
go test -race ./...
go vet ./...
```

Using a mock HTTP server, we verify API requests and errors, streaming, the CLI, document preservation, the REPL, model retrieval and caching, and server startup and shutdown. Actual model retrieval and inference are not performed during testing.

## Model Licensing

For model usage terms, please refer to the [PLaMo Community License](https://plamo.preferredai.jp/info/plamo-community-license-en).
