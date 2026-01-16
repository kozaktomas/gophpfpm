package main

import (
	"bytes"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestAccessLogger_LogFpm_Disabled(t *testing.T) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)

	config := &Config{AccessLog: false}
	accessLogger := NewAccessLogger(config, logger)

	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	resp := &ResponseData{Status: 200}

	accessLogger.LogFpm(req, resp)

	if buf.Len() > 0 {
		t.Errorf("expected no log output when AccessLog is disabled, got: %s", buf.String())
	}
}

func TestAccessLogger_LogFpm_Enabled(t *testing.T) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})

	config := &Config{AccessLog: true}
	accessLogger := NewAccessLogger(config, logger)

	reqURL, _ := url.Parse("http://example.com/api/users?page=1")
	req := &http.Request{
		Method: "GET",
		URL:    reqURL,
		Header: http.Header{
			"User-Agent": []string{"TestAgent/1.0"},
		},
	}
	resp := &ResponseData{
		Status: 200,
		Route:  "/api/users",
		Body:   []byte("response body"),
	}

	accessLogger.LogFpm(req, resp)

	output := buf.String()
	if !strings.Contains(output, "access") {
		t.Errorf("expected log to contain 'access', got: %s", output)
	}
	if !strings.Contains(output, "GET") {
		t.Errorf("expected log to contain method 'GET', got: %s", output)
	}
	if !strings.Contains(output, "200") {
		t.Errorf("expected log to contain status '200', got: %s", output)
	}
}

func TestAccessLogger_LogFpm_NilRequest(t *testing.T) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)

	config := &Config{AccessLog: true}
	accessLogger := NewAccessLogger(config, logger)

	resp := &ResponseData{Status: 200}

	// Should not panic, should log error
	accessLogger.LogFpm(nil, resp)

	output := buf.String()
	if !strings.Contains(output, "request is nil") {
		t.Errorf("expected error log about nil request, got: %s", output)
	}
}

func TestAccessLogger_LogFpm_NilRequestURL(t *testing.T) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)

	config := &Config{AccessLog: true}
	accessLogger := NewAccessLogger(config, logger)

	req := &http.Request{
		Method: "GET",
		URL:    nil,
	}
	resp := &ResponseData{Status: 200}

	// Should not panic, should log error
	accessLogger.LogFpm(req, resp)

	output := buf.String()
	if !strings.Contains(output, "URL is nil") {
		t.Errorf("expected error log about nil URL, got: %s", output)
	}
}

func TestAccessLogger_LogFpm_NilResponse(t *testing.T) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)

	config := &Config{AccessLog: true}
	accessLogger := NewAccessLogger(config, logger)

	req, _ := http.NewRequest("GET", "http://example.com/", nil)

	// Should not panic, should log error
	accessLogger.LogFpm(req, nil)

	output := buf.String()
	if !strings.Contains(output, "response is nil") {
		t.Errorf("expected error log about nil response, got: %s", output)
	}
}

func TestNewAccessLogger(t *testing.T) {
	logger := logrus.New()
	config := &Config{AccessLog: true}

	accessLogger := NewAccessLogger(config, logger)

	if accessLogger == nil {
		t.Fatal("NewAccessLogger returned nil")
	}

	if accessLogger.config != config {
		t.Error("config not properly set")
	}

	if accessLogger.logger != logger {
		t.Error("logger not properly set")
	}
}
