package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoggingResponseWriter_WriteHeader(t *testing.T) {
	recorder := httptest.NewRecorder()
	lrw := NewLoggingResponseWriter(recorder)

	lrw.WriteHeader(http.StatusCreated)

	if lrw.statusCode != http.StatusCreated {
		t.Errorf("statusCode = %d, want %d", lrw.statusCode, http.StatusCreated)
	}

	if recorder.Code != http.StatusCreated {
		t.Errorf("underlying recorder code = %d, want %d", recorder.Code, http.StatusCreated)
	}
}

func TestLoggingResponseWriter_WriteHeader_MultipleStatuses(t *testing.T) {
	tests := []int{
		http.StatusOK,
		http.StatusBadRequest,
		http.StatusInternalServerError,
		http.StatusNotFound,
		http.StatusUnauthorized,
	}

	for _, status := range tests {
		recorder := httptest.NewRecorder()
		lrw := NewLoggingResponseWriter(recorder)

		lrw.WriteHeader(status)

		if lrw.statusCode != status {
			t.Errorf("statusCode = %d, want %d", lrw.statusCode, status)
		}
	}
}

func TestLoggingResponseWriter_Write(t *testing.T) {
	recorder := httptest.NewRecorder()
	lrw := NewLoggingResponseWriter(recorder)

	body := []byte("Hello, World!")
	n, err := lrw.Write(body)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if n != len(body) {
		t.Errorf("bytes written = %d, want %d", n, len(body))
	}

	if recorder.Body.String() != string(body) {
		t.Errorf("body = %q, want %q", recorder.Body.String(), string(body))
	}
}

func TestLoggingResponseWriter_DefaultStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	lrw := NewLoggingResponseWriter(recorder)

	// Before WriteHeader is called, statusCode should be 0 (default)
	if lrw.statusCode != 0 {
		t.Errorf("initial statusCode = %d, want 0", lrw.statusCode)
	}
}

func TestLoggingResponseWriter_Header(t *testing.T) {
	recorder := httptest.NewRecorder()
	lrw := NewLoggingResponseWriter(recorder)

	lrw.Header().Set("X-Custom", "value")

	if recorder.Header().Get("X-Custom") != "value" {
		t.Errorf("header X-Custom = %q, want %q", recorder.Header().Get("X-Custom"), "value")
	}
}

func TestNewLoggingResponseWriter(t *testing.T) {
	recorder := httptest.NewRecorder()
	lrw := NewLoggingResponseWriter(recorder)

	if lrw == nil {
		t.Fatal("NewLoggingResponseWriter returned nil")
	}

	if lrw.ResponseWriter != recorder {
		t.Error("ResponseWriter not properly set")
	}

	if lrw.statusCode != 0 {
		t.Errorf("initial statusCode = %d, want 0", lrw.statusCode)
	}
}
