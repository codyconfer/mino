.PHONY: build dev install command run serve daemon test fmt fmt-check vet lint govulncheck check ci package clean icons

DIST    ?= dist
BIN     ?= $(DIST)/munin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X 'github.com/codyconfer/munin/cmd.Version=$(VERSION)'
INSTALL_DIR ?= $(shell d="$$(go env GOBIN)"; [ -n "$$d" ] || d="$$(go env GOPATH)/bin"; printf '%s' "$$d")

# Runtime + build knobs for the mode targets (command/serve/daemon/run):
#   ARGS  — forwarded verbatim to munin, e.g. `make command ARGS="fly work -o json"`
#   RACE  — set to build with the race detector, e.g. `make run RACE=1`
#   TAGS  — extra build tags, e.g. `make build TAGS=demo`
#
# TAGS=nodaemon compiles munin WITHOUT serve/daemon mode: the realtime watcher,
# its event socket, the OS service wiring, and the `serve`/`daemon` commands are
# all left out, and service-only plugin contributions (NTR reminders) stay
# hidden because nothing can attach. `deck` and every cli directive still work.
# Honored by build/dev/install/test/package, e.g. `make package TAGS=nodaemon`.
ARGS ?=
RACE ?=
TAGS ?=
GOFLAGS_TAGS := $(if $(TAGS),-tags "$(TAGS)",)
GOFLAGS_DEV := $(if $(RACE),-race,) $(GOFLAGS_TAGS)

# EMAIL_DOMAIN, when set, compiles a locked-down build that only completes
# onboarding (and thus unlocks munin) if the git signing key has a GitHub-verified
# identity in that domain, e.g. `make package EMAIL_DOMAIN=example.com`. Left empty,
# munin builds with no domain restriction.
EMAIL_DOMAIN ?=
ifneq ($(EMAIL_DOMAIN),)
LDFLAGS += -X 'github.com/codyconfer/munin/internal/app/onboard.RequiredEmailDomain=$(EMAIL_DOMAIN)'
endif

# ALL_OR_NOTHING_AUTH, when set, compiles a build where cli directives block when
# GitHub is authenticated but not fully authorized (missing signing verification,
# scope, or onboarding), instead of warning and continuing.
ALL_OR_NOTHING_AUTH ?=
ifneq ($(ALL_OR_NOTHING_AUTH),)
LDFLAGS += -X 'github.com/codyconfer/munin/internal/app/onboard.AllOrNothingAuth=true'
endif

# Regenerate the embedded system-tray / notification state icons from the raven
# SVGs in internal/render/icons/svg into internal/render/icons/data/<theme>/<state>.png.
# Uses rsvg-convert if present (best), else ImageMagick. Override size with
# ICON_SIZE=NN. SVG state names map to munin daemon states.
ICON_SVG  := internal/render/icons/svg
ICON_DATA := internal/render/icons/data
ICON_SIZE ?= 128
icons:
	@render() { \
	  if command -v rsvg-convert >/dev/null 2>&1; then \
	    rsvg-convert -w $(ICON_SIZE) -h $(ICON_SIZE) "$$1" -o "$$2"; \
	  elif command -v magick >/dev/null 2>&1; then \
	    magick -background none "$$1" -resize $(ICON_SIZE)x$(ICON_SIZE) "$$2"; \
	  else echo "need rsvg-convert (librsvg2-bin) or imagemagick"; exit 1; fi; }; \
	for theme in dark light; do \
	  out="$(ICON_DATA)/$$theme"; mkdir -p "$$out"; \
	  render "$(ICON_SVG)/munin--$$theme--dimmed.svg"      "$$out/inactive.png"; \
	  render "$(ICON_SVG)/munin--$$theme--standard.svg"    "$$out/running.png"; \
	  render "$(ICON_SVG)/munin--$$theme--highlighted.svg" "$$out/notify.png"; \
	  render "$(ICON_SVG)/munin--$$theme--warning.svg"     "$$out/warn.png"; \
	  render "$(ICON_SVG)/munin--$$theme--error.svg"       "$$out/error.png"; \
	done; \
	echo "wrote $(ICON_DATA)/{dark,light}/{inactive,running,notify,warn,error}.png"

# Supported target matrix. Fixed by the prebuilt duckdb-go-bindings shipped in
# go.mod: adding a platform here without a matching bindings package won't link.
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

# Local OS/arch matrix (same set as package). Prefer `make package` for binaries;
# `matrix` is a CI-doc alias that lists platforms and builds the host package.
matrix:
	@echo "platforms: $(PLATFORMS)"
	@$(MAKE) package

# Build all packages for the host.
build:
	go build $(GOFLAGS_TAGS) ./...

# Build a dev binary to $(BIN) honoring RACE/TAGS/EMAIL_DOMAIN/ALL_OR_NOTHING_AUTH.
# Phony so it always rebuilds (go build is incremental) before a mode target runs.
dev:
	@mkdir -p $(dir $(BIN))
	@go build $(GOFLAGS_DEV) -ldflags "$(LDFLAGS)" -o $(BIN) .

# Install the host munin binary to $(INSTALL_DIR)/munin, replacing any existing
# binary. Honors the same RACE/TAGS/EMAIL_DOMAIN/ALL_OR_NOTHING_AUTH knobs as `dev`.
# This is the Go toolchain PATH install — not `munin install` (config provision)
# and not `make daemon` (OS service).
install:
	@mkdir -p "$(INSTALL_DIR)"
	@go build $(GOFLAGS_DEV) -ldflags "$(LDFLAGS)" -o "$(INSTALL_DIR)/munin" .
	@echo "installed $(INSTALL_DIR)/munin"

# cli mode: run a directive and print formatted output. e.g.
#   make command ARGS="fly work -o json"
command: dev
	@$(BIN) $(ARGS)

# serve mode: foreground realtime watcher in the CURRENT shell (Ctrl-C exits).
# No OS integration; logs stream to the shell and the log dir. e.g.
#   make serve ARGS="work --interval 30s"
serve: dev
	@$(BIN) serve $(ARGS)

# daemon mode: install munin as an OS service if needed, then start it (idempotent).
#   make daemon ARGS="work"
daemon: dev
	@$(BIN) daemon $(ARGS)

# deck mode: the interactive TUI (attaches to a running daemon, else starts a
# silent background serve provider owned by the deck session — it stops when
# deck exits). e.g. `make run` or `make run ARGS="work"`.
run: dev
	@$(BIN) deck $(ARGS)

# Cross-compile release binaries for every supported platform/arch into $(DIST).
#
# munin links DuckDB via cgo, so cross-builds need a C cross-compiler for each
# target. `zig cc` provides one for all targets from any host; install zig
# (https://ziglang.org) to build the full matrix. The host target always builds
# with the native compiler, so `make package` works without zig for that one and
# skips the others with a notice.
package:
	@mkdir -p $(DIST)
	@host="$$(go env GOOS)/$$(go env GOARCH)"; \
	have_zig=0; command -v zig >/dev/null 2>&1 && have_zig=1; \
	for p in $(PLATFORMS); do \
	  os=$${p%/*}; arch=$${p#*/}; \
	  out="$(DIST)/munin_$(VERSION)_$${os}_$${arch}"; \
	  [ "$$os" = windows ] && out="$$out.exe"; \
	  cc=""; \
	  if [ "$$p" != "$$host" ]; then \
	    if [ $$have_zig -eq 1 ]; then \
	      case "$$p" in \
	        darwin/amd64)  cc="zig cc -target x86_64-macos" ;; \
	        darwin/arm64)  cc="zig cc -target aarch64-macos" ;; \
	        linux/amd64)   cc="zig cc -target x86_64-linux-gnu" ;; \
	        linux/arm64)   cc="zig cc -target aarch64-linux-gnu" ;; \
	        windows/amd64) cc="zig cc -target x86_64-windows-gnu" ;; \
	      esac; \
	    else \
	      echo "skip $$p (cross-cgo needs a C cross-compiler; install zig)"; \
	      continue; \
	    fi; \
	  fi; \
	  echo "build $$p -> $$out"; \
	  if [ -n "$$cc" ]; then \
	    CGO_ENABLED=1 GOOS=$$os GOARCH=$$arch CC="$$cc" \
	      go build -trimpath $(GOFLAGS_TAGS) -ldflags "$(LDFLAGS)" -o "$$out" . || exit 1; \
	  else \
	    CGO_ENABLED=1 GOOS=$$os GOARCH=$$arch \
	      go build -trimpath $(GOFLAGS_TAGS) -ldflags "$(LDFLAGS)" -o "$$out" . || exit 1; \
	  fi; \
	done; \
	( cd $(DIST) && (sha256sum munin_* 2>/dev/null || shasum -a 256 munin_*) > SHA256SUMS ); \
	echo "artifacts written to $(DIST)/"

# Remove build artifacts.
clean:
	rm -rf $(DIST)

# Run the test suite.
test:
	go test $(GOFLAGS_TAGS) ./...

# Tooling lives in ./tools (separate module) so consumers don't inherit linter deps.
GO_TOOL = go tool -modfile=tools/go.mod

# Format all Go source in place (gofmt + goimports via golangci-lint).
fmt:
	$(GO_TOOL) golangci-lint fmt

# Verify all Go source is formatted; fail (showing the diff) if not.
fmt-check:
	$(GO_TOOL) golangci-lint fmt --diff

# go vet: the standard toolchain analyzers.
vet:
	go vet ./...

# golangci-lint: aggregate static analysis (govet, staticcheck, errcheck, ...).
lint:
	$(GO_TOOL) golangci-lint run

# govulncheck: report known vulnerabilities in dependencies and reachable code.
govulncheck:
	$(GO_TOOL) govulncheck ./...

# Full gate: build, format check, lint, vulncheck, test.
check: build fmt-check lint govulncheck test

# CI entrypoint: identical to the full gate.
ci: check
