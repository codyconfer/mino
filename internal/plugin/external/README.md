# External plugins graduated (M6)

Lane C / C2 packages (`gcx`, `kubectl`, `gooseai`, `pi`, `opencode`, `ollama`,
and the shared `stub` helper) no longer ship in the stock munin binary.

**Local overlay layout (siblings of this repo):**

```text
../munin-plugins-external/     public-SDK plugin packages + tests
../munin-overlay-template/     thin binary: RegisterPlugins → externals.Register
```

Build stock munin without externals:

```sh
make build
go test ./...
```

Build with externals:

```sh
cd ../munin-overlay-template && make build
```

See [`docs/m6-distribution-kit.md`](../../../docs/m6-distribution-kit.md).
