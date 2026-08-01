# External plugins

Nothing here ships in the stock mino binary.

**In-repo overlay module** — the Google (Calendar, Gmail, Docs, Drive, Tasks),
Slack, and demo signals:

```text
external/plugins/            public-SDK plugin packages + tests
external/plugins/overlay/    thin binary: RegisterPlugins → plugins.Register
```

```sh
make build-overlay
make test-overlay
cd external/plugins && go run ./overlay calendar query
```

**Sibling overlay repos** — Lane C / C2 packages (`gcx`, `kubectl`, `gooseai`,
`pi`, `opencode`, `ollama`, and the shared `stub` helper):

```text
../mino-plugins-external/     public-SDK plugin packages + tests
../mino-overlay-template/     thin binary: RegisterPlugins → externals.Register
```

Build stock mino without either:

```sh
make build
go test ./...
```

See [`external/plugins/README.md`](../../../external/plugins/README.md) and
[`docs/readme.md`](../../../docs/readme.md#plugins--notes).
