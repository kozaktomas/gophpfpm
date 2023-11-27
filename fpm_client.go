package main

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
	"strings"
	"time"
)

type FpmClient struct {
	fCgiClient *FCgiClient
	config     *Config
	monitor    *Monitor
	logger     *logrus.Logger
}

// ResponseData struct contains encapsulated data from fpm response
type ResponseData struct {
	Status  int
	Headers map[string][]string
	Body    []byte
	Route   string // parse route from FPM response header X-App-Route
}

func NewFpmClient(fCgiClient *FCgiClient, config *Config, monitor *Monitor, logger *logrus.Logger) *FpmClient {
	return &FpmClient{
		fCgiClient: fCgiClient,
		config:     config,
		monitor:    monitor,
		logger:     logger,
	}
}

func (fpm *FpmClient) Call(request *http.Request) (*ResponseData, error) {
	requestBody, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read request body: %w", err)
	}

	params := map[string]string{
		"SCRIPT_FILENAME": fpm.config.IndexFile,
		"SERVER_SOFTWARE": "gophpfpm/1.0.0",
		"SERVER_NAME":     request.Host,
		"SERVER_PORT":     fmt.Sprintf("%d", fpm.config.Port),
		"REQUEST_URI":     request.URL.RequestURI(),
		"QUERY_STRING":    request.URL.Query().Encode(),
		"REQUEST_METHOD":  request.Method,
		"CONTENT_TYPE":    request.Header.Get("Content-type"),
	}
	// propagate http request headers through params
	for name, headers := range request.Header {
		for _, header := range headers {
			h := strings.ToLower(name)
			// do not propagate protected headers
			_, found := protectedHeadersInbound[h]
			if !found {
				params[fmt.Sprintf("HTTP_%s", strings.ToUpper(name))] = header
			}
		}
	}

	fpmReq := fpm.fCgiClient.NewRequest(params, nil)
	// set request body
	if len(requestBody) > 0 {
		fpmReq.Body = requestBody
	}

	start := time.Now()
	fpmResp, err := fpm.fCgiClient.SendRequest(fpmReq)
	if err != nil {
		fpm.monitor.FmpDurationHistogram.
			WithLabelValues(
				fpm.config.App,
				TypeFpm,
				request.Method,
				fmt.Sprintf("%d", 0),
				"",
			).
			Observe(float64(time.Since(start)))
		return nil, fmt.Errorf("could not call FPM: %w", err)
	}
	route := fpmResp.Header.Get("X-App-Route")
	fpm.monitor.FmpDurationHistogram.
		WithLabelValues(
			fpm.config.App,
			TypeFpm,
			request.Method,
			fmt.Sprintf("%d", fpmResp.StatusCode),
			route,
		).
		Observe(time.Since(start).Seconds())

	// read data from response
	body, err := io.ReadAll(fpmResp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body: %w", err)
	}

	return &ResponseData{
		Status:  fpmResp.StatusCode,
		Headers: fpmResp.Header,
		Body:    body,
		Route:   route,
	}, nil
}

func (fpm *FpmClient) Close() {
	fpm.fCgiClient.Close()
}
