package serve

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codyconfer/sisyphus/stream"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

// freePort returns a port nothing is listening on.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", net.JoinHostPort(config.HTTPLoopback, "0"))
	if err != nil {
		t.Fatalf("probing for a free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func TestAPIDisabledOpensNoListener(t *testing.T) {
	s, _ := testServer(t, t.TempDir())
	ln, err := s.apiListener(RunOptions{})
	if err != nil {
		t.Fatalf("apiListener with HTTP off: %v", err)
	}
	if ln != nil {
		_ = ln.Close()
		t.Fatal("a listener was opened with the API off; serve must not bind a port nobody asked for")
	}
}

func TestAPIListenerBindsLoopbackByDefault(t *testing.T) {
	s, _ := testServer(t, t.TempDir())
	ln, err := s.apiListener(RunOptions{HTTP: true, HTTPPort: freePort(t)})
	if err != nil {
		t.Fatalf("apiListener: %v", err)
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address is %T, want *net.TCPAddr", ln.Addr())
	}
	if !addr.IP.IsLoopback() {
		t.Errorf("bound %s with no HTTPHost set, which is not loopback; an unset host must never "+
			"widen the bind", addr.IP)
	}
}

func TestAPIListenerHonoursTheConfiguredHost(t *testing.T) {
	s, _ := testServer(t, t.TempDir())
	ln, err := s.apiListener(RunOptions{HTTP: true, HTTPHost: "::1", HTTPPort: freePort(t)})
	if err != nil {
		t.Fatalf("apiListener on ::1: %v", err)
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address is %T, want *net.TCPAddr", ln.Addr())
	}
	if addr.IP.To4() != nil {
		t.Errorf("bound %s for HTTPHost ::1; the host was ignored", addr.IP)
	}
}

func TestAPIListenerFailsOnAnUnbindableHost(t *testing.T) {
	s, _ := testServer(t, t.TempDir())
	ln, err := s.apiListener(RunOptions{HTTP: true, HTTPHost: "192.0.2.1", HTTPPort: freePort(t)})
	if ln != nil {
		_ = ln.Close()
		t.Fatal("bound 192.0.2.1, an address this machine does not hold")
	}
	if err == nil {
		t.Fatal("apiListener on an unbindable host = nil; a host that cannot be bound must fail loudly, " +
			"not leave the caller with no listener and no error")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %v, want KindConfig; the host came from config or a flag", errs.KindOf(err))
	}
	if !strings.Contains(errs.Hint(err), "--http-host") {
		t.Errorf("hint = %q; want --http-host named so the user knows which setting to change", errs.Hint(err))
	}
}

func TestAPIListenerFailsWhenThePortIsTaken(t *testing.T) {
	port := freePort(t)
	held, err := net.Listen("tcp", net.JoinHostPort(config.HTTPLoopback, strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("holding the port: %v", err)
	}
	defer held.Close()

	s, _ := testServer(t, t.TempDir())
	ln, err := s.apiListener(RunOptions{HTTP: true, HTTPPort: port})
	if ln != nil {
		_ = ln.Close()
	}
	// Unlike the unix socket, a taken TCP port may belong to an unrelated
	// program, and the API was explicitly asked for. Degrading silently would
	// leave the caller staring at "connection refused" with no explanation.
	if err == nil {
		t.Fatal("binding a taken port succeeded")
	}
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("kind = %q, want config", errs.KindOf(err))
	}
	if errs.Hint(err) == "" {
		t.Error("no hint naming --http-port or daemon.http.port")
	}
}

func TestRunFailsBeforeOpeningSourcesWhenThePortIsTaken(t *testing.T) {
	port := freePort(t)
	held, err := net.Listen("tcp", net.JoinHostPort(config.HTTPLoopback, strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("holding the port: %v", err)
	}
	defer held.Close()

	s, _ := testServer(t, t.TempDir())
	err = s.Run(context.Background(), RunOptions{
		Flight: "nope", HTTP: true, HTTPPort: port, HTTPToken: "token-long-enough-here",
	})
	if err == nil {
		t.Fatal("Run succeeded with a taken API port")
	}
	// The flight does not exist either, so if the port were checked after
	// sources() we would see that error instead. Binding first means nothing is
	// half-started when the port is unavailable.
	if errs.KindOf(err) != errs.KindConfig {
		t.Errorf("error = %v (kind %q); want the bind failure, which proves the port is claimed before "+
			"any source is opened", err, errs.KindOf(err))
	}
}

func TestEndSessionShutsTheAPIDownFirst(t *testing.T) {
	s, st := testServer(t, t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	subj := stream.NewSubject[signals.Event]()

	var apiClosed, joined, socketClosed atomic.Bool
	flightID := st.StartFlight("serve", "test")
	audited := make(chan struct{})
	close(audited)

	s.endSession(session{
		cancel: cancel,
		closeAPI: func() {
			if ctx.Err() == nil {
				t.Error("the API was shut down before the context was cancelled, so in-flight handlers " +
					"would not have been told to stop")
			}
			apiClosed.Store(true)
		},
		src: sources{join: func() {
			if !apiClosed.Load() {
				t.Error("sources joined before the API shut down; an in-flight run would still be writing " +
					"audit rows after the flight rolled up")
			}
			joined.Store(true)
		}},
		closeSock: func() {
			if !joined.Load() {
				t.Error("socket closed before the sources were joined")
			}
			socketClosed.Store(true)
		},
		subj:     subj,
		audited:  audited,
		flightID: flightID,
	})

	if !apiClosed.Load() || !joined.Load() || !socketClosed.Load() {
		t.Fatalf("teardown incomplete: apiClosed=%v joined=%v socketClosed=%v",
			apiClosed.Load(), joined.Load(), socketClosed.Load())
	}
}

func TestHTTPAPIServesAndShutsDown(t *testing.T) {
	s, _ := testServer(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	port := freePort(t)
	ln, err := s.apiListener(RunOptions{HTTP: true, HTTPPort: port})
	if err != nil {
		t.Fatalf("apiListener: %v", err)
	}
	subj := stream.NewSubject[signals.Event]()
	defer subj.Close()

	stop := s.httpAPI(ctx, ln, subj, RunOptions{
		Flight: "default", HTTP: true, HTTPPort: port,
		HTTPToken: "token-long-enough-here", HTTPTokenSource: "test",
	}, nil)

	addr := ln.Addr().String()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("the API is not accepting connections: %v", err)
	}
	_ = conn.Close()

	cancel()
	stop()

	// After Shutdown the port must be free again, or a restart cannot rebind.
	reln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("the port is still held after shutdown: %v", err)
	}
	_ = reln.Close()
}

func TestHTTPAPIWithNoListenerIsANoOp(t *testing.T) {
	s, _ := testServer(t, t.TempDir())
	subj := stream.NewSubject[signals.Event]()
	defer subj.Close()
	stop := s.httpAPI(context.Background(), nil, subj, RunOptions{}, nil)
	if stop == nil {
		t.Fatal("httpAPI returned a nil closer, which endSession would dereference")
	}
	stop() // must not panic
}

func TestAPIConfigRedactsSecretsWithoutMutatingTheLiveConfig(t *testing.T) {
	s, _ := testServer(t, t.TempDir())
	s.Cfg.Daemon.HTTP.Token = "http-token-0123456789"
	s.Cfg.GitHub.ServiceToken = "ghp_service_secret"
	s.Cfg.GitHub.App.PrivateKeyPath = "/run/secrets/app.pem"

	got, ok := s.apiConfig().(config.Config)
	if !ok {
		t.Fatalf("apiConfig() = %T, want config.Config", s.apiConfig())
	}
	if got.Daemon.HTTP.Token != "<set>" {
		t.Errorf("http token = %q, want <set>", got.Daemon.HTTP.Token)
	}
	if got.GitHub.ServiceToken != "<set>" {
		t.Errorf("github.service_token = %q, want <set>; /api/v1/config is reachable by anyone holding "+
			"the API bearer token, which is a strictly weaker credential than a GitHub PAT",
			got.GitHub.ServiceToken)
	}
	if got.GitHub.App.PrivateKeyPath == "" {
		t.Error("the key path was blanked; it is a diagnostic, not a secret — only key material is")
	}

	if s.Cfg.GitHub.ServiceToken != "ghp_service_secret" {
		t.Errorf("apiConfig mutated the live config: service_token = %q. It shallow-copies *s.Cfg, so a "+
			"pointer field would let redaction destroy the running configuration",
			s.Cfg.GitHub.ServiceToken)
	}
	if s.Cfg.Daemon.HTTP.Token != "http-token-0123456789" {
		t.Error("apiConfig mutated the live http token")
	}
}
