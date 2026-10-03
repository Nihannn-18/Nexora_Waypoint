package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// startServe runs serve on a loopback port with handler and returns the base
// URL, the cancel that stands in for SIGTERM, and serve's result channel.
func startServe(t *testing.T, handler http.Handler, drain time.Duration) (string, context.CancelFunc, <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	go func() { done <- serve(ctx, &http.Server{Handler: handler}, ln, drain) }()
	return "http://" + ln.Addr().String(), cancel, done
}

func TestServeDrainsInFlightRequestOnShutdown(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		_, _ = io.WriteString(w, "finished")
	})
	base, cancel, done := startServe(t, handler, 5*time.Second)

	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		resp, err := http.Get(base)
		if err != nil {
			got <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		got <- result{body: string(b), err: err}
	}()

	<-entered
	cancel() // the shutdown signal arrives mid-request

	select {
	case err := <-done:
		t.Fatalf("serve returned %v before the in-flight request finished", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	if r := <-got; r.err != nil || r.body != "finished" {
		t.Fatalf("in-flight request = %q, %v; want it to complete", r.body, r.err)
	}
	if err := <-done; err != nil {
		t.Fatalf("serve = %v, want a clean shutdown", err)
	}

	if _, err := http.Get(base); err == nil {
		t.Fatal("server still accepts connections after shutdown")
	}
}

func TestServeGivesUpAfterDrainTimeout(t *testing.T) {
	entered := make(chan struct{})
	stuck := make(chan struct{})
	t.Cleanup(func() { close(stuck) })
	handler := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-stuck
	})
	base, cancel, done := startServe(t, handler, 50*time.Millisecond)

	go func() {
		if resp, err := http.Get(base); err == nil {
			resp.Body.Close()
		}
	}()
	<-entered
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("serve = nil, want an error when requests outlive the drain timeout")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve hung instead of closing connections after the drain timeout")
	}
}

func TestServeReturnsServerError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ln.Close() // Serve on a closed listener fails immediately

	if err := serve(context.Background(), &http.Server{}, ln, time.Second); err == nil {
		t.Fatal("serve = nil, want the listener error")
	}
}
