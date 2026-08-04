package httpapi

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/codyconfer/sisyphus/stream"

	"github.com/codyconfer/mino/internal/signals"
)

// subjectDeps wires Subscribe to a real stream.Subject, so the tests exercise the
// same drop-on-full fan-out serve uses.
func subjectDeps(t *testing.T) (Deps, *stream.Subject[signals.Event]) {
	t.Helper()
	subj := stream.NewSubject[signals.Event]()
	t.Cleanup(subj.Close)
	d := fakeDeps()
	d.Subscribe = func(buffer int) (<-chan signals.Event, func()) {
		ch := subj.Subscribe(buffer)
		return ch, func() { subj.Unsubscribe(ch) }
	}
	d.Encode = func(ev signals.Event) ([]byte, error) {
		return []byte(`{"source":"` + ev.Source + `"}`), nil
	}
	return d, subj
}

// eventStream is a live SSE connection. It deliberately does not expose the
// *http.Response: the body must stay open for the stream's lifetime, and
// openStream already closes it via t.Cleanup.
type eventStream struct {
	header http.Header
	br     *bufio.Reader
	cancel context.CancelFunc
}

func openStream(t *testing.T, client *http.Client, url string) eventStream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET", url+"/api/v1/events", nil)
	if err != nil {
		cancel()
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	res, err := client.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("GET /v1/events: %v", err)
	}
	t.Cleanup(func() { cancel(); _ = res.Body.Close() })
	return eventStream{header: res.Header, br: bufio.NewReader(res.Body), cancel: cancel}
}

// readFrame reads until a blank line, returning the frame's lines.
func readFrame(t *testing.T, br *bufio.Reader) []string {
	t.Helper()
	var lines []string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading an sse frame: %v (got %v)", err, lines)
		}
		line = strings.TrimRight(line, "\n")
		if line == "" {
			return lines
		}
		lines = append(lines, line)
	}
}

func TestEventsStreamsPublishedEvents(t *testing.T) {
	d, subj := subjectDeps(t)
	srv := testAPI(t, d)
	es := openStream(t, srv.Client(), srv.URL)
	br := es.br

	if got := es.header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if es.header.Get("Content-Length") != "" {
		t.Error("Content-Length was set on a stream with no known length")
	}

	// The preamble proves the connection is live before any event arrives.
	if got := readFrame(t, br); got[0] != "retry: 5000" {
		t.Errorf("first frame = %v, want a retry hint", got)
	}
	if got := readFrame(t, br); got[0] != ": connected" {
		t.Errorf("second frame = %v, want the connected comment", got)
	}

	// Subscribe happens before the preamble, so the subscriber is registered.
	subj.Next(signals.Event{Source: "demo", At: time.Now()})

	frame := readFrame(t, br)
	joined := strings.Join(frame, "\n")
	for _, want := range []string{"id: 1", "event: signal", `data: {"source":"demo"}`} {
		if !strings.Contains(joined, want) {
			t.Errorf("event frame missing %q:\n%s", want, joined)
		}
	}
}

func TestEventsSendsHeartbeats(t *testing.T) {
	orig := sseHeartbeat
	sseHeartbeat = 20 * time.Millisecond
	t.Cleanup(func() { sseHeartbeat = orig })

	d, _ := subjectDeps(t)
	srv := testAPI(t, d)
	br := openStream(t, srv.Client(), srv.URL).br
	readFrame(t, br) // retry
	readFrame(t, br) // connected

	// Without a heartbeat an idle stream is indistinguishable from a dead one.
	if got := readFrame(t, br); got[0] != ": ping" {
		t.Errorf("frame = %v, want a ping comment", got)
	}
}

func TestEventsUnsubscribesWhenTheClientDisconnects(t *testing.T) {
	d, subj := subjectDeps(t)
	api := New(Config{Token: testToken, TokenSource: "test", MaxConcurrent: 2}, d)
	srv := newTestServer(t, api)

	es := openStream(t, srv.Client(), srv.URL)
	readFrame(t, es.br)
	readFrame(t, es.br)
	if got := api.SSEClients(); got != 1 {
		t.Fatalf("SSEClients = %d, want 1", got)
	}

	es.cancel()
	// A leaked subscriber is a permanent tax on every later publish, and it holds
	// an SSE slot forever.
	deadline := time.Now().Add(5 * time.Second)
	for api.SSEClients() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("the SSE slot was never released after the client disconnected")
		}
		time.Sleep(5 * time.Millisecond)
	}
	subj.Next(signals.Event{Source: "after"}) // must not panic or block
}

func TestEventsStopsWhenTheSubjectCloses(t *testing.T) {
	subj := stream.NewSubject[signals.Event]()
	d := fakeDeps()
	d.Subscribe = func(buffer int) (<-chan signals.Event, func()) {
		ch := subj.Subscribe(buffer)
		return ch, func() { subj.Unsubscribe(ch) }
	}
	api := New(Config{Token: testToken, TokenSource: "test", MaxConcurrent: 2}, d)
	srv := newTestServer(t, api)

	br := openStream(t, srv.Client(), srv.URL).br
	readFrame(t, br)
	readFrame(t, br)

	subj.Close() // what endSession does at shutdown
	deadline := time.Now().Add(5 * time.Second)
	for api.SSEClients() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("the handler did not return when the Subject closed, so serve shutdown would hang on it")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestEventsRejectsBeyondTheConnectionCap(t *testing.T) {
	d, _ := subjectDeps(t)
	srv := testAPI(t, d)

	var cancels []context.CancelFunc
	t.Cleanup(func() {
		for _, c := range cancels {
			c()
		}
	})
	for range maxSSEConns {
		es := openStream(t, srv.Client(), srv.URL)
		cancels = append(cancels, es.cancel)
		readFrame(t, es.br)
		readFrame(t, es.br)
	}

	res := do(t, srv, "GET", "/api/v1/events", testToken, nil)
	if res.status != http.StatusTooManyRequests {
		t.Errorf("connection %d = %d, want 429; every subscriber costs a send on stream.Subject's single "+
			"fan-out goroutine", maxSSEConns+1, res.status)
	}
}

func TestAStalledSSEClientDoesNotBlockPublishing(t *testing.T) {
	d, subj := subjectDeps(t)
	srv := testAPI(t, d)
	// Open a stream and never read from it.
	_ = openStream(t, srv.Client(), srv.URL)

	// stream.Subject drops for a full subscriber rather than blocking its
	// publisher. If that ever changed, one wedged curl would stall the whole
	// serve loop, which is the single worst failure this feature could add.
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range eventBuffer * 4 {
			subj.Next(signals.Event{Source: "flood", At: time.Now(), Section: signals.Section{Signal: "s", Title: string(rune('a' + i%26))}})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("publishing stalled behind an SSE client that stopped reading")
	}
	wg.Wait()
}

func TestSSEFlushErrorReachesTheConnectionThroughGin(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	type probe struct{ unwrapped, direct bool }
	got := make(chan probe, 1)

	r := gin.New()
	r.GET("/probe", func(c *gin.Context) {
		_, unwrapped := unwrapWriter(c.Writer).(interface{ FlushError() error })
		_, direct := c.Writer.(interface{ FlushError() error })
		got <- probe{unwrapped: unwrapped, direct: direct}
		c.Status(http.StatusOK)
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	res, err := srv.Client().Get(srv.URL + "/probe")
	if err != nil {
		t.Fatalf("GET /probe: %v", err)
	}
	res.Body.Close()

	p := <-got
	if !p.unwrapped {
		t.Error("unwrapWriter did not reach a writer with FlushError; ResponseController.Flush matches " +
			"http.Flusher before it unwraps, so on gin's writer it calls the error-less Flush and always " +
			"returns nil — the SSE broken-peer check would never fire and a dead peer would hold a slot " +
			"until it gave up, with WriteTimeout deliberately zero")
	}
	if p.direct {
		t.Error("gin's ResponseWriter gained FlushError, so unwrapWriter is no longer needed for Flush; " +
			"simplify here, not in the SSE handler")
	}
}
