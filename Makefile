.PHONY: build run test vet staticcheck govulncheck lint check package clean

DIST    ?= dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w

# Supported target matrix. Fixed by the prebuilt duckdb-go-bindings shipped in
# go.mod: adding a platform here without a matching bindings package won't link.
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

# Build all packages for the host.
build:
	go build ./...

# Run munin: opens the interactive TUI.
run:
	go run . tui

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

# go vet: the standard toolchain analyzers.
vet:
	go vet ./...

# staticcheck: open-source static analysis (honnef.co/go/tools).
staticcheck:
	go tool staticcheck ./...

# govulncheck: report known vulnerabilities in dependencies and reachable code.
govulncheck:
	go tool govulncheck ./...

# All static analysis: vet + staticcheck + govulncheck.
lint: vet staticcheck govulncheck

# Full gate: build, lint, test.
check: build lint test
