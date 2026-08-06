package kubectl

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
)

const (
	DefaultBinary  = "kubectl"
	DefaultTimeout = 10 * time.Second
)

type runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

type execRunner struct {
	binary  string
	timeout time.Duration
}

func newExecRunner(binary string, timeout time.Duration) *execRunner {
	if binary == "" {
		binary = DefaultBinary
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &execRunner{binary: binary, timeout: timeout}
}

func (r *execRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	out, err := exec.CommandContext(cctx, r.binary, args...).Output()
	if err == nil {
		return out, nil
	}
	wrapped := errx.Wrapf(err, "%s %s", r.binary, strings.Join(args, " "))
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		wrapped = wrapped.WithHint("%s", errx.ExcerptBytes(ee.Stderr))
	}
	return nil, wrapped
}

func binaryAvailable(binary string) bool {
	if binary == "" {
		binary = DefaultBinary
	}
	_, err := exec.LookPath(binary)
	return err == nil
}

func binaryName(binary string) string {
	if binary == "" {
		return DefaultBinary
	}
	return binary
}

type scope struct {
	context       string
	namespace     string
	allNamespaces bool
	timeout       time.Duration
}

func (s scope) globalArgs() []string {
	out := make([]string, 0, 4)
	if s.context != "" {
		out = append(out, "--context", s.context)
	}
	if s.timeout > 0 {
		out = append(out, "--request-timeout", s.timeout.String())
	}
	return out
}

func (s scope) selectorArgs() []string {
	switch {
	case s.allNamespaces:
		return []string{"--all-namespaces"}
	case s.namespace != "":
		return []string{"--namespace", s.namespace}
	default:
		return nil
	}
}

func (s scope) clusterScoped() scope {
	s.namespace = ""
	s.allNamespaces = false
	return s
}

func get(ctx context.Context, r runner, s scope, resource string, extra ...string) ([]byte, error) {
	args := append(s.globalArgs(), "get", resource, "-o", "json")
	args = append(args, s.selectorArgs()...)
	return r.Run(ctx, append(args, extra...)...)
}
