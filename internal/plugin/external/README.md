# External plugins

Nothing here ships in the stock mino binary.

**In-repo overlay module** — the Google (Calendar, Gmail, Docs, Drive, Tasks),
Slack, and demo signals, plus the Lane C / C2 packages (`gcx`, `kubectl`,
`ollama`, `argocd`, the shared `stub` helper, and the `example` overlay sample):

```text
external/plugins/            public-SDK plugin packages + tests
external/plugins/overlay/    thin binary: RegisterPlugins → plugins.Register,
                             embedded overlay/defaults/ seed tree
```

```sh
make build-overlay
make test-overlay
cd external/plugins && go run ./overlay calendar query
```

Build stock mino without it:

```sh
make build
go test ./...
```

See [`external/plugins/README.md`](../../../external/plugins/README.md) and
[`docs/plugins.md`](../../../docs/plugins.md).
