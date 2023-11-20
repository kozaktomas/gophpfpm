package main

import (
	"context"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type HttpServer struct {
	Port int

	fpmClient *FpmClient
	srv       *http.Server
	config    *Config
	logger    *logrus.Logger
}

// LoggingResponseWriter is a wrapper around an http.ResponseWriter that
// allows you to capture the status code written to the response.
type LoggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

// NewLoggingResponseWriter creates a new LoggingResponseWriter
func NewLoggingResponseWriter(w http.ResponseWriter) *LoggingResponseWriter {
	return &LoggingResponseWriter{w, 0}
}

// WriteHeader captures the status code written to the response
func (lrw *LoggingResponseWriter) WriteHeader(code int) {
	lrw.ResponseWriter.WriteHeader(code)
	lrw.statusCode = code
}

func NewHttpServer(config *Config, fpmClient *FpmClient, monitor *Monitor, logger *logrus.Logger) *HttpServer {
	router := http.NewServeMux()

	// public files
	// todo: optional
	// todo: configuration with folders
	staticMiddleWare := func(endpointPrefix string, next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			lrw := NewLoggingResponseWriter(w)
			next.ServeHTTP(lrw, r)
			monitor.HttpDurationHistogram.
				WithLabelValues(
					config.App,
					TypeHttp,
					r.Method,
					fmt.Sprintf("%d", lrw.statusCode),
					fmt.Sprintf("%s<asset>", endpointPrefix),
				).
				Observe(time.Since(start).Seconds())
		})
	}

	for _, staticFolder := range config.StaticFolders {
		parts := strings.Split(staticFolder, ":")
		if len(parts) != 2 {
			logger.Fatalf("invalid static folder definition: %s", staticFolder)
		}
		fs := http.FileServer(http.Dir(parts[0]))
		prefix := fmt.Sprintf("%s/", parts[1])
		router.Handle(prefix, staticMiddleWare(prefix, http.StripPrefix(parts[1], fs)))
	}

	// prometheus metrics handler
	router.Handle("/metrics", promhttp.HandlerFor(
		monitor.Registry,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
			Registry:          monitor.Registry,
		},
	))

	// default route to handle anything else
	router.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		start := time.Now()
		fpmResponse, err := fpmClient.Call(request)

		if err != nil {
			logger.Errorf("could not call FPM: %s\n", err)
			writer.WriteHeader(http.StatusInternalServerError)
			_, writeError := writer.Write([]byte("Internal server error"))
			if writeError != nil {
				// should not happen
				logger.Errorf("could not write response body: %s\n", err)
			}
			monitor.HttpDurationHistogram.
				WithLabelValues(
					config.App,
					TypeHttp,
					request.Method,
					fmt.Sprintf("%d", http.StatusInternalServerError),
					"",
				).
				Observe(time.Since(start).Seconds())
			return
		}

		for name, headers := range fpmResponse.Headers {
			for _, header := range headers {
				_, found := protectedHeadersOutbound[strings.ToLower(name)]
				if !found {
					writer.Header().Add(name, header)
				}
			}
		}

		writer.WriteHeader(fpmResponse.Status)
		_, err = writer.Write(fpmResponse.Body)
		if err != nil {
			// should not happen
			logger.Errorf("could not write response body: %s\n", err)
			return
		}

		monitor.HttpDurationHistogram.
			WithLabelValues(
				config.App,
				TypeHttp,
				request.Method,
				fmt.Sprintf("%d", fpmResponse.Status),
				fpmResponse.Route,
			).
			Observe(time.Since(start).Seconds())
	})

	return &HttpServer{
		Port:      config.Port,
		fpmClient: fpmClient,
		srv: &http.Server{
			Addr:    fmt.Sprintf(":%d", config.Port),
			Handler: router,
		},
		config: config,
		logger: logger,
	}
}

func (hs *HttpServer) StartServer() {
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := hs.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			hs.logger.Infof("listen: %s\n", err)
		}
	}()
	hs.logger.Info("Server Started")

	<-done
	hs.logger.Info("Server Stopped")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		// extra handling here
		cancel()
	}()

	if err := hs.srv.Shutdown(ctx); err != nil {
		hs.logger.Fatalf("Server Shutdown Failed:%+v", err)
	}

	hs.fpmClient.Close()

	hs.logger.Info("Server Exited Properly")
}
