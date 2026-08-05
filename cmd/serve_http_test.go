package cmd

import (
	"slices"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
)

func useServeHTTPTestApp(t *testing.T, enabled bool, port int) {
	t.Helper()
	useServeHTTPTestAppOn(t, enabled, config.DefaultHTTPHost, port)
}

func useServeHTTPTestAppOn(t *testing.T, enabled bool, host string, port int) {
	t.Helper()
	orig := shared
	t.Cleanup(func() { shared = orig })
	cfg := config.Defaults()
	cfg.Home = t.TempDir()
	cfg.Daemon.HTTP.Enabled = enabled
	cfg.Daemon.HTTP.Host = host
	cfg.Daemon.HTTP.Port = port
	shared = &app.App{Cfg: cfg, Directives: &config.Directives{}}
	closeSharedDBs(t)
}

func TestServeRefusesAPrivilegedHTTPPortFlag(t *testing.T) {
	useServeHTTPTestApp(t, false, 0)
	for _, raw := range []string{"0", "80", "443", "1023", "70000"} {
		err := runServe(t, "--http", "--http-port", raw)
		if err == nil {
			t.Errorf("serve --http --http-port %s = nil; a privileged or out-of-range port is a "+
				"misconfiguration, not something to attempt", raw)
			continue
		}
		if !strings.Contains(err.Error(), "--http-port") {
			t.Errorf("serve --http-port %s error = %q; want the flag named so the user knows what to change",
				raw, err)
		}
	}
}

func TestServeRefusesAPrivilegedHTTPPortFromConfig(t *testing.T) {
	useServeHTTPTestApp(t, true, 80)
	err := runServe(t)
	if err == nil {
		t.Fatal("serve with daemon.http.port=80 = nil; the config path bypassed the flag, so the range has " +
			"to be checked on the resolved value")
	}
	if !strings.Contains(err.Error(), "daemon.http.port") {
		t.Errorf("error = %q; want daemon.http.port named, since no flag was passed", err)
	}
}

func TestServeIgnoresTheHTTPPortWhenTheAPIIsOff(t *testing.T) {
	// The port only matters if something is going to bind it. Validating it
	// unconditionally would break every existing serve invocation the moment a
	// stale port sat in config.
	useServeHTTPTestApp(t, false, 80)
	err := runServe(t)
	if err != nil && strings.Contains(err.Error(), "http-port") {
		t.Errorf("serve without --http was rejected over the port: %v", err)
	}
}

func TestServeAcceptsAnUnprivilegedHTTPPort(t *testing.T) {
	useServeHTTPTestApp(t, false, 0)
	for _, raw := range []string{"1024", "7717", "65535"} {
		err := runServe(t, "--http", "--http-port", raw)
		if err != nil && strings.Contains(err.Error(), "http-port") {
			t.Errorf("serve --http-port %s was rejected: %v", raw, err)
		}
	}
}

func TestServeRefusesAnInvalidHTTPHostFlag(t *testing.T) {
	useServeHTTPTestApp(t, false, 7717)
	for _, raw := range []string{"", "http://x/", "127.0.0.1:7717", "a b"} {
		err := runServe(t, "--http", "--http-host", raw)
		if err == nil {
			t.Errorf("serve --http --http-host %q = nil; a value net.Listen could never accept is a "+
				"misconfiguration, not something to attempt", raw)
			continue
		}
		if !strings.Contains(err.Error(), "--http-host") {
			t.Errorf("serve --http-host %q error = %q; want the flag named so the user knows what to change",
				raw, err)
		}
	}
}

func TestServeRefusesAnInvalidHTTPHostFromConfig(t *testing.T) {
	useServeHTTPTestAppOn(t, true, "http://x/", 7717)
	err := runServe(t)
	if err == nil {
		t.Fatal("serve with daemon.http.host=http://x/ = nil; the config path bypassed the flag, so the " +
			"value has to be checked after resolution")
	}
	if !strings.Contains(err.Error(), "daemon.http.host") {
		t.Errorf("error = %q; want daemon.http.host named, since no flag was passed", err)
	}
}

func TestServeIgnoresTheHTTPHostWhenTheAPIIsOff(t *testing.T) {
	useServeHTTPTestAppOn(t, false, "http://x/", 7717)
	err := runServe(t)
	if err != nil && strings.Contains(err.Error(), "http-host") {
		t.Errorf("serve without --http was rejected over the host: %v", err)
	}
}

func TestServeAcceptsCommonHTTPHosts(t *testing.T) {
	useServeHTTPTestApp(t, false, 7717)
	for _, raw := range []string{"127.0.0.1", "0.0.0.0", "::", "::1", "localhost", "mino-container"} {
		err := runServe(t, "--http", "--http-host", raw)
		if err != nil && strings.Contains(err.Error(), "http-host") {
			t.Errorf("serve --http-host %s was rejected: %v", raw, err)
		}
	}
}

func TestSelfServeArgsDisablesTheHTTPAPI(t *testing.T) {
	useServeHTTPTestApp(t, true, 7717)
	args := selfServeArgs()
	// deck spawns this provider to feed the socket. If it inherited
	// daemon.http.enabled it would try to bind a port a foreground serve may
	// already hold, and the hard bind failure would kill deck's provider.
	if !slices.Contains(args, "--http=false") {
		t.Errorf("selfServeArgs() = %v; want --http=false so the deck-owned provider never claims the port", args)
	}
}
