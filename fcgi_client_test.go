package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeFpm is a minimal FastCGI responder on a unix socket. It records the
// BEGIN_REQUEST flags of every request and whether the client closed each
// connection after the response.
type fakeFpm struct {
	listener net.Listener

	mu          sync.Mutex
	flags       []byte
	connections int
	closedByCli int
	wg          sync.WaitGroup
}

func newFakeFpm(t *testing.T) (*fakeFpm, string) {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "fpm.sock")
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeFpm{listener: l}
	go f.serve()
	t.Cleanup(func() { _ = l.Close() })
	return f, socket
}

func (f *fakeFpm) serve() {
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.connections++
		f.mu.Unlock()
		f.wg.Add(1)
		go f.handle(conn)
	}
}

func (f *fakeFpm) handle(conn net.Conn) {
	defer f.wg.Done()
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	var requestId uint16
	// read records until the empty FCGI_STDIN that ends the request
	for {
		h := FCgiRecord{}
		if err := binary.Read(conn, binary.BigEndian, &h); err != nil {
			return
		}
		content := make([]byte, int(h.ContentLength)+int(h.PaddingLength))
		if _, err := io.ReadFull(conn, content); err != nil {
			return
		}
		requestId = h.RequestId
		if h.Type == FCGI_BEGIN_REQUEST {
			f.mu.Lock()
			f.flags = append(f.flags, content[2])
			f.mu.Unlock()
		}
		if h.Type == FCGI_STDIN && h.ContentLength == 0 {
			break
		}
	}

	w := &FCgiConnection{Conn: conn}
	_ = w.writeRecord(requestId, FCGI_STDOUT, []byte("Content-Type: text/plain\r\n\r\nok"))
	_ = w.writeRecord(requestId, FCGI_END_REQUEST, make([]byte, 8))

	// a client that does not reuse the connection closes it now
	if _, err := conn.Read(make([]byte, 1)); errors.Is(err, io.EOF) {
		f.mu.Lock()
		f.closedByCli++
		f.mu.Unlock()
	}
}

func TestFCgiClient_NewConnectionPerRequestWithoutKeepConn(t *testing.T) {
	fpm, socket := newFakeFpm(t)
	config := newTestConfig()
	config.Socket = socket
	config.FpmPoolSize = 2
	config.Timeout = 5 * time.Second

	client, err := NewFCgiClient(config, newTestLogger())
	if err != nil {
		t.Fatalf("NewFCgiClient: %v", err)
	}

	const requests = 3
	for i := 0; i < requests; i++ {
		req := client.NewRequest(map[string]string{"REQUEST_METHOD": "POST"}, []byte("payload"))
		resp, err := client.SendRequest(req)
		if err != nil {
			t.Fatalf("SendRequest %d: %v", i, err)
		}
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Equal(body, []byte("ok")) {
			t.Errorf("body = %q, want %q", body, "ok")
		}
	}
	client.Close()
	_ = fpm.listener.Close()
	fpm.wg.Wait()

	fpm.mu.Lock()
	defer fpm.mu.Unlock()
	// +1 for the startup probe connection in NewFCgiClient
	if fpm.connections != requests+1 {
		t.Errorf("connections = %d, want %d (one per request + probe)", fpm.connections, requests+1)
	}
	// the probe closes before sending anything, so only requests are counted here
	if fpm.closedByCli != requests {
		t.Errorf("connections closed by client after the response = %d, want %d", fpm.closedByCli, requests)
	}
	if len(fpm.flags) != requests {
		t.Fatalf("BEGIN_REQUEST records = %d, want %d", len(fpm.flags), requests)
	}
	for i, fl := range fpm.flags {
		if fl != 0 {
			t.Errorf("request %d: BEGIN_REQUEST flags = %d, want 0 (no FCGI_KEEP_CONN)", i, fl)
		}
	}
}

func TestNewFCgiClient_Errors(t *testing.T) {
	config := newTestConfig()
	config.Socket = filepath.Join(t.TempDir(), "missing.sock")
	config.FpmPoolSize = 1
	if _, err := NewFCgiClient(config, newTestLogger()); err == nil {
		t.Error("expected error for unreachable socket")
	}

	config.FpmPoolSize = 0
	if _, err := NewFCgiClient(config, newTestLogger()); err == nil {
		t.Error("expected error for pool size 0")
	}
}
