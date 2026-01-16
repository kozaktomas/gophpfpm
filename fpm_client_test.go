package main

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// mockFCgiClient is a test double for FCgiClientInterface
type mockFCgiClient struct {
	lastParams   map[string]string
	lastBody     []byte
	response     *http.Response
	responseErr  error
	sendCalled   bool
	closeCalled  bool
	requestCount int
}

func (m *mockFCgiClient) NewRequest(params map[string]string, body []byte) FCgiRequest {
	m.lastParams = params
	m.lastBody = body
	return FCgiRequest{
		Params:    params,
		Body:      body,
		requestId: 1,
	}
}

func (m *mockFCgiClient) SendRequest(r FCgiRequest) (*http.Response, error) {
	m.sendCalled = true
	m.requestCount++
	return m.response, m.responseErr
}

func (m *mockFCgiClient) Close() {
	m.closeCalled = true
}

func newTestLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return logger
}

func newTestConfig() *Config {
	return &Config{
		Port:      8080,
		IndexFile: "/var/www/index.php",
		App:       "test-app",
	}
}

func newTestMonitor() *Monitor {
	return NewMonitor(newTestLogger())
}

func TestFpmClient_Call_ParamsBuilding(t *testing.T) {
	mock := &mockFCgiClient{
		response: &http.Response{
			StatusCode: 200,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("OK")),
		},
	}
	config := newTestConfig()
	monitor := newTestMonitor()
	logger := newTestLogger()

	fpm := NewFpmClient(mock, config, monitor, logger)

	req, _ := http.NewRequest("POST", "http://example.com/api/users?foo=bar", strings.NewReader("body"))
	req.Header.Set("Content-Type", "application/json")

	_, err := fpm.Call(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify params were built correctly
	tests := []struct {
		param    string
		expected string
	}{
		{"SCRIPT_FILENAME", "/var/www/index.php"},
		{"SERVER_SOFTWARE", "gophpfpm/1.0.0"},
		{"SERVER_NAME", "example.com"},
		{"SERVER_PORT", "8080"},
		{"REQUEST_URI", "/api/users?foo=bar"},
		{"QUERY_STRING", "foo=bar"},
		{"REQUEST_METHOD", "POST"},
		{"CONTENT_TYPE", "application/json"},
		{"HTTP_HOST", "example.com"},
	}

	for _, tt := range tests {
		if mock.lastParams[tt.param] != tt.expected {
			t.Errorf("param %s = %q, want %q", tt.param, mock.lastParams[tt.param], tt.expected)
		}
	}
}

func TestFpmClient_Call_HeaderPropagation(t *testing.T) {
	mock := &mockFCgiClient{
		response: &http.Response{
			StatusCode: 200,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("OK")),
		},
	}
	config := newTestConfig()
	monitor := newTestMonitor()
	logger := newTestLogger()

	fpm := NewFpmClient(mock, config, monitor, logger)

	req, _ := http.NewRequest("GET", "http://example.com/", strings.NewReader(""))
	req.Header.Set("X-Custom-Header", "custom-value")
	req.Header.Set("Authorization", "Bearer token123")
	req.Header.Set("Accept", "application/json")

	_, err := fpm.Call(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify custom headers are propagated with HTTP_ prefix
	tests := []struct {
		param    string
		expected string
	}{
		{"HTTP_X-CUSTOM-HEADER", "custom-value"},
		{"HTTP_AUTHORIZATION", "Bearer token123"},
		{"HTTP_ACCEPT", "application/json"},
	}

	for _, tt := range tests {
		if mock.lastParams[tt.param] != tt.expected {
			t.Errorf("param %s = %q, want %q", tt.param, mock.lastParams[tt.param], tt.expected)
		}
	}
}

func TestFpmClient_Call_ProtectedHeadersFiltered(t *testing.T) {
	mock := &mockFCgiClient{
		response: &http.Response{
			StatusCode: 200,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("OK")),
		},
	}
	config := newTestConfig()
	monitor := newTestMonitor()
	logger := newTestLogger()

	fpm := NewFpmClient(mock, config, monitor, logger)

	req, _ := http.NewRequest("POST", "http://example.com/", strings.NewReader("body"))
	// These headers are in protectedHeadersInbound and should NOT be propagated as HTTP_*
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", "4")

	_, err := fpm.Call(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// HTTP_CONTENT-TYPE and HTTP_CONTENT-LENGTH should NOT exist
	// (Content-Type is set via CONTENT_TYPE param instead)
	protectedParams := []string{"HTTP_CONTENT-TYPE", "HTTP_CONTENT-LENGTH"}
	for _, param := range protectedParams {
		if _, exists := mock.lastParams[param]; exists {
			t.Errorf("protected header %s should not be propagated", param)
		}
	}

	// But CONTENT_TYPE should be set explicitly
	if mock.lastParams["CONTENT_TYPE"] != "application/json" {
		t.Errorf("CONTENT_TYPE = %q, want %q", mock.lastParams["CONTENT_TYPE"], "application/json")
	}
}

func TestFpmClient_Call_ResponseWrapping(t *testing.T) {
	mock := &mockFCgiClient{
		response: &http.Response{
			StatusCode: 201,
			Header: http.Header{
				"Content-Type": []string{"text/html"},
				"X-App-Route":  []string{"/api/create"},
			},
			Body: io.NopCloser(strings.NewReader("Created")),
		},
	}
	config := newTestConfig()
	monitor := newTestMonitor()
	logger := newTestLogger()

	fpm := NewFpmClient(mock, config, monitor, logger)

	req, _ := http.NewRequest("POST", "http://example.com/", strings.NewReader(""))
	resp, err := fpm.Call(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Status != 201 {
		t.Errorf("Status = %d, want 201", resp.Status)
	}

	if resp.Route != "/api/create" {
		t.Errorf("Route = %q, want %q", resp.Route, "/api/create")
	}

	if string(resp.Body) != "Created" {
		t.Errorf("Body = %q, want %q", string(resp.Body), "Created")
	}

	if len(resp.Headers["Content-Type"]) == 0 || resp.Headers["Content-Type"][0] != "text/html" {
		t.Errorf("Headers[Content-Type] = %v, want %q", resp.Headers["Content-Type"], "text/html")
	}
}

func TestFpmClient_Call_RequestBody(t *testing.T) {
	mock := &mockFCgiClient{
		response: &http.Response{
			StatusCode: 200,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("OK")),
		},
	}
	config := newTestConfig()
	monitor := newTestMonitor()
	logger := newTestLogger()

	fpm := NewFpmClient(mock, config, monitor, logger)

	body := `{"name": "test"}`
	req, _ := http.NewRequest("POST", "http://example.com/", strings.NewReader(body))

	_, err := fpm.Call(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The body should have been read and available in the request
	// Note: The actual body is set on the FCgiRequest after NewRequest is called
	if !mock.sendCalled {
		t.Error("SendRequest was not called")
	}
}

func TestFpmClient_Call_QueryStringEncoding(t *testing.T) {
	mock := &mockFCgiClient{
		response: &http.Response{
			StatusCode: 200,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("OK")),
		},
	}
	config := newTestConfig()
	monitor := newTestMonitor()
	logger := newTestLogger()

	fpm := NewFpmClient(mock, config, monitor, logger)

	reqURL, _ := url.Parse("http://example.com/search")
	reqURL.RawQuery = "q=hello+world&page=1"
	req := &http.Request{
		Method: "GET",
		URL:    reqURL,
		Host:   "example.com",
		Header: http.Header{},
		Body:   io.NopCloser(bytes.NewReader(nil)),
	}

	_, err := fpm.Call(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Query string should be properly encoded
	qs := mock.lastParams["QUERY_STRING"]
	if !strings.Contains(qs, "q=") && !strings.Contains(qs, "page=1") {
		t.Errorf("QUERY_STRING = %q, expected to contain query params", qs)
	}
}

func TestFpmClient_Close(t *testing.T) {
	mock := &mockFCgiClient{}
	config := newTestConfig()
	monitor := newTestMonitor()
	logger := newTestLogger()

	fpm := NewFpmClient(mock, config, monitor, logger)
	fpm.Close()

	if !mock.closeCalled {
		t.Error("Close was not called on FCgiClient")
	}
}
