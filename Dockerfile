# syntax=docker/dockerfile:1

# mino links DuckDB through cgo. The bindings static-link DuckDB itself but link
# libstdc++, libgcc, libm and glibc dynamically and pass -rdynamic, so scratch,
# distroless/static-* and distroless/base-* cannot run the result.
#
# BUILDER AND RUNTIME MUST BE THE SAME DEBIAN RELEASE. The builder's glibc sets
# the binary's symbol floor: a trixie build dies on a bookworm runtime with
# "version GLIBC_2.x not found". Bump both or neither.
ARG GO_VERSION=1.26.5
ARG DEBIAN_RELEASE=bookworm

FROM golang:${GO_VERSION}-${DEBIAN_RELEASE} AS build

# GOWORK=off because go.work uses ../sisyphus and ../viewkit, which do not exist
# in a build context. go.mod pins the published versions and go.sum has both.
ENV CGO_ENABLED=1 \
    GOWORK=off \
    GOFLAGS=-trimpath

WORKDIR /src
COPY . .

# TAGS is deliberately empty. NOT nodaemon: sisyphus mode.Supported(ModeServe)
# returns the DaemonSupported constant, false under that tag, and cmd/gate.go runs
# mode.Gate on every command, so `mino serve` would fail with "unsupported by this
# build". Only -tags daemon links kardianos/service and fyne.io/systray, so the
# default build already excludes them. ALL_OR_NOTHING_AUTH is never set: it flips
# the gate to PolicyBlock, and with no keyring and no settings.yaml every start
# would become an onboarding error.
#
# VERSION arrives as a build arg because .dockerignore excludes .git.
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build \
    make dev BIN=/out/mino VERSION="${VERSION}" SERVICE_AUTH=1 \
 && ldd /out/mino


FROM debian:${DEBIAN_RELEASE}-slim AS runtime

ARG UID=10001
ARG GID=10001

# debian-slim ships libgcc-s1 but not libstdc++6, which the duckdb bindings need.
# ca-certificates is required for the GitHub signal. curl exists only for
# HEALTHCHECK. No tini: `mino serve` never forks (ensure_unix.go's startSilent is
# reached only from the deck) and installs its own SIGTERM handler, so it is a
# correct PID 1. Use `init: true` / --init if you run the deck in a container.
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates \
      libstdc++6 \
      curl \
 && rm -rf /var/lib/apt/lists/*

RUN groupadd --gid "${GID}" mino \
 && useradd --uid "${UID}" --gid "${GID}" --home-dir /var/lib/mino \
            --no-create-home --shell /usr/sbin/nologin mino

COPY --from=build /out/mino /usr/local/bin/mino

# MINO_HOME must be set: config.Home falls through to os.UserHomeDir and errors
# when neither it nor $HOME resolves. XDG_CONFIG_HOME lives inside MINO_HOME so
# settings.yaml lands on the volume; the directive walker skips dot-directories,
# so .config is never read as directives.
#
# MINO_DAEMON_BELL=false as env, not a flag: daemon.bell defaults true and would
# write BEL bytes into `docker logs`, and env survives a CMD override.
ENV MINO_HOME=/var/lib/mino \
    HOME=/var/lib/mino \
    XDG_CONFIG_HOME=/var/lib/mino/.config \
    MINO_LOG_LEVEL=info \
    MINO_LOG_COLOR=never \
    MINO_OUTPUT=json \
    MINO_DAEMON_BELL=false \
    MINO_DAEMON_DESKTOP=false \
    MINO_DAEMON_HTTP_ENABLED=true \
    MINO_DAEMON_HTTP_HOST=0.0.0.0 \
    MINO_DAEMON_HTTP_PORT=7717

# Docker chowns an EMPTY named volume to match the image directory it covers, so
# creating this tree as mino is what lets USER mino write with no entrypoint chown.
# A bind mount gets no such treatment; see docker-compose.yaml. No VOLUME
# instruction on purpose: it would force an anonymous volume on every docker run.
RUN mkdir -p /var/lib/mino/.data /var/lib/mino/logs /var/lib/mino/.config \
 && chown -R "${UID}:${GID}" /var/lib/mino \
 && chmod 0700 /var/lib/mino

COPY --chmod=0755 <<'EOF' /usr/local/bin/mino-entrypoint
#!/bin/sh
set -eu

# `mino serve` resolves its flight from the directives in MINO_HOME and hard errors
# with `no flight named "default"` against an empty one. Nothing auto-provisions.
# `mino install` skips the onboarding gate and never touches the keyring, so it is
# safe unattended. No --force: that would clobber an edited config.yaml on restart.
if [ ! -e "$MINO_HOME/config.yaml" ] \
&& [ ! -e "$MINO_HOME/config.yml" ] \
&& [ ! -e "$MINO_HOME/config.json" ]; then
  echo "mino: provisioning $MINO_HOME (first run)" >&2
  mino install
fi

if [ "${1-}" = "mino" ]; then shift; fi

exec mino "$@"
EOF

EXPOSE 7717

# /healthz is the only unauthenticated route and is exempt from the Host check, so
# no token is needed. It exists only while the HTTP API is on: with
# MINO_DAEMON_HTTP_ENABLED=false, disable this healthcheck too or the container
# flaps unhealthy forever.
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
  CMD curl -fsS "http://127.0.0.1:${MINO_DAEMON_HTTP_PORT}/healthz" >/dev/null || exit 1

USER mino:mino
WORKDIR /var/lib/mino

ENTRYPOINT ["/usr/local/bin/mino-entrypoint"]
CMD ["serve"]

LABEL org.opencontainers.image.title="mino" \
      org.opencontainers.image.description="mino serve with the HTTP trigger API" \
      org.opencontainers.image.source="https://github.com/codyconfer/mino"
