.PHONY: build run test fmt fmt-check vet lint govulncheck check ci package clean icons

DIST    ?= dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w

# EMAIL_DOMAIN, when set, compiles a locked-down build that only completes
# onboarding (and thus unlocks munin) if the git signing key has a GitHub-verified
# identity in that domain, e.g. `make package EMAIL_DOMAIN=grafana.com`. Left empty,
# munin builds with no domain restriction.
EMAIL_DOMAIN ?=
ifneq ($(EMAIL_DOMAIN),)
LDFLAGS += -X 'github.com/codyconfer/munin/internal/onboard.RequiredEmailDomain=$(EMAIL_DOMAIN)'
endif

# Regenerate the embedded system-tray / notification state icons from the raven
# SVGs in internal/icons/svg into internal/icons/data/<theme>/<state>.png.
# Uses rsvg-convert if present (best), else ImageMagick. Override size with
# ICON_SIZE=NN. SVG state names map to munin daemon states.
ICON_SVG  := internal/icons/svg
ICON_DATA := internal/icons/data
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

# Build all packages for the host.
build:
	go build ./...

# Run munin's TUI, streaming stderr to a SECOND terminal window so logs aren't
# swallowed by the alt-screen. Honors $TERMINAL, else tries common emulators
# (Linux) or Terminal.app (macOS); falls back to a temp file if none is found.
run:
	@err="$$(mktemp "$${TMPDIR:-/tmp}/munin-stderr.XXXXXX")"; \
	tailcmd="tail -n +1 -f '$$err'"; \
	launch() { \
	  if [ -n "$$TERMINAL" ] && command -v "$$TERMINAL" >/dev/null 2>&1; then "$$TERMINAL" -e sh -c "$$tailcmd" >/dev/null 2>&1 & return 0; fi; \
	  if command -v gnome-terminal >/dev/null 2>&1; then gnome-terminal --title=munin-stderr -- sh -c "$$tailcmd" >/dev/null 2>&1 & return 0; fi; \
	  if command -v kitty >/dev/null 2>&1; then kitty --title munin-stderr sh -c "$$tailcmd" >/dev/null 2>&1 & return 0; fi; \
	  if command -v wezterm >/dev/null 2>&1; then wezterm start -- sh -c "$$tailcmd" >/dev/null 2>&1 & return 0; fi; \
	  for t in konsole x-terminal-emulator alacritty foot xterm; do \
	    command -v $$t >/dev/null 2>&1 && { $$t -e sh -c "$$tailcmd" >/dev/null 2>&1 & return 0; }; \
	  done; \
	  if command -v osascript >/dev/null 2>&1; then \
	    osascript -e "tell application \"Terminal\" to do script \"$$tailcmd\"" >/dev/null 2>&1 & return 0; fi; \
	  return 1; \
	}; \
	if launch; then echo "munin: stderr -> new terminal window ($$err)"; \
	else echo "munin: no terminal emulator found; stderr -> $$err (run: tail -f $$err)"; fi; \
	trap 'rm -f "$$err"' EXIT INT TERM; \
	go run . tui 2>"$$err"

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
	      go build -trimpath -ldflags "$(LDFLAGS)" -o "$$out" . || exit 1; \
	  else \
	    CGO_ENABLED=1 GOOS=$$os GOARCH=$$arch \
	      go build -trimpath -ldflags "$(LDFLAGS)" -o "$$out" . || exit 1; \
	  fi; \
	done; \
	( cd $(DIST) && (sha256sum munin_* 2>/dev/null || shasum -a 256 munin_*) > SHA256SUMS ); \
	echo "artifacts written to $(DIST)/"

# Remove build artifacts.
clean:
	rm -rf $(DIST)

# Run the test suite.
test:
	go test ./...

# Format all Go source in place (gofmt + goimports via golangci-lint).
fmt:
	go tool golangci-lint fmt

# Verify all Go source is formatted; fail (showing the diff) if not.
fmt-check:
	go tool golangci-lint fmt --diff

# go vet: the standard toolchain analyzers.
vet:
	go vet ./...

# golangci-lint: aggregate static analysis (govet, staticcheck, errcheck, ...).
lint:
	go tool golangci-lint run

# govulncheck: report known vulnerabilities in dependencies and reachable code.
govulncheck:
	go tool govulncheck ./...

# Full gate: build, format check, lint, vulncheck, test.
check: build fmt-check lint govulncheck test

# CI entrypoint: identical to the full gate.
ci: check
