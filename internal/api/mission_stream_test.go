package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
)

// ssePipeWriter adapts an io.Pipe into the ResponseWriter+Flusher the stream
// handler needs, so tests exercise it in-process with no sockets. io.Pipe
// provides backpressure: the handler blocks in Write until the test reads.
type ssePipeWriter struct {
	pr *io.PipeReader
	pw *io.PipeWriter
}

func (w *ssePipeWriter) Header() http.Header         { return http.Header{} }
func (w *ssePipeWriter) WriteHeader(int)             {}
func (w *ssePipeWriter) Write(p []byte) (int, error) { return w.pw.Write(p) }
func (w *ssePipeWriter) Flush()                      {}

// noFlushWriter is an http.ResponseWriter without a Flush method.
type noFlushWriter struct {
	hdr    http.Header
	status int
	body   bytes.Buffer
}

func (w *noFlushWriter) Header() http.Header         { return w.hdr }
func (w *noFlushWriter) WriteHeader(code int)        { w.status = code }
func (w *noFlushWriter) Write(p []byte) (int, error) { return w.body.Write(p) }

// startStream starts handleStreamMission in a goroutine and returns the
// pipe reader to consume events from, plus a cancel func and a done channel.
func startStream(s *Server) (*io.PipeReader, context.CancelFunc, <-chan struct{}) {
	pr, pw := io.Pipe()
	w := &ssePipeWriter{pr: pr, pw: pw}

	req := httptest.NewRequest(http.MethodGet, "/api/mission/stream", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer pw.Close()
		s.handleStreamMission(w, req)
	}()
	return pr, cancel, done
}

// readEvent reads one SSE frame ("event: snapshot\ndata: <json>\n\n") and
// returns the decoded snapshot. It blocks until the frame arrives, so tests
// synchronize on real data instead of sleeping.
func readEvent(sc *bufio.Scanner) (MissionSnapshot, error) {
	if !sc.Scan() {
		return MissionSnapshot{}, fmt.Errorf("expected event frame: %v", sc.Err())
	}
	if got := sc.Text(); got != "event: snapshot" {
		return MissionSnapshot{}, fmt.Errorf("event line = %q; want %q", got, "event: snapshot")
	}
	if !sc.Scan() {
		return MissionSnapshot{}, fmt.Errorf("expected data line: %v", sc.Err())
	}
	var snap MissionSnapshot
	if err := json.Unmarshal([]byte(strings.TrimPrefix(sc.Text(), "data: ")), &snap); err != nil {
		return MissionSnapshot{}, fmt.Errorf("data line is not a MissionSnapshot JSON: %w", err)
	}
	if !sc.Scan() || sc.Text() != "" {
		return MissionSnapshot{}, fmt.Errorf("expected blank line terminating the SSE frame")
	}
	return snap, nil
}

// newStreamScanner wraps a pipe reader in a scanner with a generous buffer.
func newStreamScanner(pr io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	return sc
}

func TestHandleGetMission_MatchesMissionSnapshot(t *testing.T) {
	s := New(&config.Config{
		FirstName:       "Ada",
		LastName:        "Lovelace",
		Email:           "ada@example.com",
		ResumePath:      "/tmp/resume.pdf",
		TargetJobTitles: "Engineer",
		TargetLocations: "Remote",
		ApplyConsent:    true,
		MaxAppsPerDay:   7,
	}, nil, nil, "")

	rec := httptest.NewRecorder()
	s.handleGetMission(rec, httptest.NewRequest(http.MethodGet, "/api/mission", nil))

	// Decode both sides so byte-level artifacts (writeJSON's trailing newline)
	// don't mask a real difference in the snapshot content.
	var viaHTTP, direct MissionSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &viaHTTP); err != nil {
		t.Fatalf("GET /api/mission did not return a MissionSnapshot: %v", err)
	}
	directBytes, err := json.Marshal(s.missionSnapshot())
	if err != nil {
		t.Fatalf("missionSnapshot() did not marshal: %v", err)
	}
	if err := json.Unmarshal(directBytes, &direct); err != nil {
		t.Fatalf("missionSnapshot() did not unmarshal: %v", err)
	}
	if !reflect.DeepEqual(viaHTTP, direct) {
		t.Fatalf("GET /api/mission body != missionSnapshot():\n%+v\n%+v", viaHTTP, direct)
	}
}

func TestWriteMissionEvent_FrameFormat(t *testing.T) {
	s := New(&config.Config{}, nil, nil, "")
	var buf bytes.Buffer
	if err := writeMissionEvent(&buf, s.missionSnapshot()); err != nil {
		t.Fatalf("writeMissionEvent() error: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "event: snapshot\n") {
		t.Fatalf("frame = %q; want it to start with %q", got, "event: snapshot\n")
	}
	if !strings.Contains(got, "\ndata: {") {
		t.Fatalf("frame = %q; want a data line with a JSON object", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Fatalf("frame = %q; want it to end with a blank line", got)
	}
	// The payload must be on a single data: line — SSE forbids raw newlines.
	if strings.Count(got, "\n") != 3 {
		t.Fatalf("frame = %q; want exactly 3 newlines (event, data, blank)", got)
	}
}

func TestStreamMission_PushesSnapshotOnChange(t *testing.T) {
	s := New(&config.Config{}, nil, nil, "")
	pr, cancel, done := startStream(s)
	defer pr.Close()
	sc := newStreamScanner(pr)

	// Initial snapshot arrives immediately on connect.
	snap, err := readEvent(sc)
	if err != nil {
		t.Fatalf("initial event: %v", err)
	}
	if snap.EngineStatus != string(StatusIdle) {
		t.Fatalf("EngineStatus = %q; want %q", snap.EngineStatus, StatusIdle)
	}

	// A state change pushes a fresh snapshot.
	s.mu.Lock()
	s.status = StatusRunning
	s.mu.Unlock()
	s.changed()

	snap, err = readEvent(sc)
	if err != nil {
		t.Fatalf("change event: %v", err)
	}
	if snap.EngineStatus != string(StatusRunning) {
		t.Fatalf("EngineStatus = %q; want %q", snap.EngineStatus, StatusRunning)
	}

	// Cancelling the request ends the stream.
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not stop after request cancellation")
	}
}

func TestStreamMission_HeartbeatPushesSnapshot(t *testing.T) {
	s := New(&config.Config{}, nil, nil, "")
	s.sseHeartbeat = 20 * time.Millisecond
	pr, cancel, done := startStream(s)
	defer pr.Close()
	sc := newStreamScanner(pr)

	if _, err := readEvent(sc); err != nil {
		t.Fatalf("initial event: %v", err)
	}

	// No state change — the next snapshot must arrive from the heartbeat,
	// proving a client that missed a wake-up re-syncs on its own.
	got := make(chan error, 1)
	go func() {
		_, err := readEvent(sc)
		got <- err
	}()
	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("heartbeat event: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("heartbeat snapshot did not arrive")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not stop after request cancellation")
	}
}

func TestStreamMission_RejectsNonFlusher(t *testing.T) {
	s := New(&config.Config{}, nil, nil, "")
	rec := &noFlushWriter{hdr: make(http.Header)}
	req := httptest.NewRequest(http.MethodGet, "/api/mission/stream", nil)
	s.handleStreamMission(rec, req)

	if rec.status != http.StatusInternalServerError {
		t.Fatalf("status = %d; want %d", rec.status, http.StatusInternalServerError)
	}
	if !strings.Contains(rec.body.String(), "streaming unsupported") {
		t.Fatalf("body = %q; want a streaming unsupported error", rec.body.String())
	}
}

func TestChanged_WakesSubscriber(t *testing.T) {
	s := New(&config.Config{}, nil, nil, "")

	// No subscribers: changed() must not block or panic.
	s.changed()

	sub := s.subscribe()
	defer s.unsubscribe(sub)

	select {
	case <-sub:
		t.Fatal("subscriber notified before any change")
	default:
	}

	s.changed()
	select {
	case <-sub:
	default:
		t.Fatal("subscriber not notified after change")
	}

	// A burst of changes with a slow consumer must not block changed():
	// wake-ups beyond the one-slot buffer drop, and because every snapshot is
	// full-state, a dropped wake-up is harmless.
	s.changed()
	s.changed()
	select {
	case <-sub:
	case <-time.After(5 * time.Second):
		t.Fatal("subscriber did not receive a wake-up")
	}

	// After unsubscribe, no more wake-ups arrive.
	s.unsubscribe(sub)
	select {
	case <-sub:
		t.Fatal("subscriber notified after unsubscribe")
	default:
	}
}
