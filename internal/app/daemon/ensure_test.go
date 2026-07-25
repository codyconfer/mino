package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestStartSilentStop(t *testing.T) {
	bin, lookErr := exec.LookPath("sleep")
	if lookErr != nil {
		if runtime.GOOS == "windows" {
			t.Skip("no sleep(1) on Windows")
		}
		t.Fatal(lookErr)
	}
	owned, err := startSilent(bin, "60")
	if err != nil {
		t.Fatalf("startSilent: %v", err)
	}
	pid := owned.proc.Pid
	if !processAlive(pid) {
		t.Fatalf("child pid %d not alive after start", pid)
	}
	owned.Stop()
	select {
	case <-owned.done:
	case <-time.After(3 * time.Second):
		t.Fatal("child did not exit after Stop")
	}
	if processAlive(pid) {
		t.Fatalf("child pid %d still alive after Stop", pid)
	}
}

func TestLifelineServeHelper(t *testing.T) {
	if os.Getenv("MUNIN_TEST_LIFELINE_SERVE") != "1" {
		return
	}
	ctx, cancel := BindDeckLifeline(context.Background())
	defer cancel()
	<-ctx.Done()
	os.Exit(0)
}

func TestStartSilentLifelineParentDeath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("lifeline pipe not wired on Windows")
	}
	if os.Getenv("MUNIN_TEST_LIFELINE_PARENT") == "1" {
		os.Setenv("MUNIN_TEST_LIFELINE_SERVE", "1")
		owned, err := startSilent(os.Args[0], "-test.run=^TestLifelineServeHelper$", "-test.count=1")
		if err != nil {
			fmt.Fprintf(os.Stderr, "startSilent: %v\n", err)
			os.Exit(2)
		}
		fmt.Print(owned.proc.Pid)
		os.Exit(0)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestStartSilentLifelineParentDeath$", "-test.count=1")
	cmd.Env = append(os.Environ(), "MUNIN_TEST_LIFELINE_PARENT=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("helper: %v\n%s", err, out)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || pid <= 0 {
		t.Fatalf("bad helper pid %q: %v", out, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for processAlive(pid) {
		if time.Now().After(deadline) {
			if p, ferr := os.FindProcess(pid); ferr == nil {
				_ = p.Kill()
			}
			t.Fatalf("orphaned child pid %d still alive after parent exit", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestBindDeckLifelineCancelsOnPipeClose(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	t.Setenv(deckLifelineEnv, strconv.Itoa(int(r.Fd())))

	ctx, cancel := BindDeckLifeline(context.Background())
	defer cancel()

	_ = w.Close()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("context did not cancel after lifeline close")
	}
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil
}
