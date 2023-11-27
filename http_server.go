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

	router       *http.ServeMux
	fpmClient    *FpmClient
	srv          *http.Server
	config       *Config
	accessLogger *AccessLogger
	monitor      *Monitor
	logger       *logrus.Logger
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

func NewHttpServer(
	config *Config,
	fpmClient *FpmClient,
	accessLogger *AccessLogger,
	monitor *Monitor,
	logger *logrus.Logger,
) *HttpServer {
	router := http.NewServeMux()

	return &HttpServer{
		Port:      config.Port,
		router:    router,
		fpmClient: fpmClient,
		srv: &http.Server{
			Addr:    fmt.Sprintf(":%d", config.Port),
			Handler: router,
		},
		config:       config,
		accessLogger: accessLogger,
		monitor:      monitor,
		logger:       logger,
	}
}

func (hs *HttpServer) PrepareServer() {
	staticMiddleWare := func(endpointPrefix string, next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			lrw := NewLoggingResponseWriter(w)
			next.ServeHTTP(lrw, r)
			hs.monitor.HttpDurationHistogram.
				WithLabelValues(
					hs.config.App,
					TypeHttp,
					r.Method,
					fmt.Sprintf("%d", lrw.statusCode),
					fmt.Sprintf("%s<asset>", endpointPrefix),
				).
				Observe(time.Since(start).Seconds())
		})
	}

	for _, staticFolder := range hs.config.StaticFolders {
		parts := strings.Split(staticFolder, ":")
		if len(parts) != 2 {
			hs.logger.Fatalf("invalid static folder definition: %s", staticFolder)
		}
		fs := http.FileServer(http.Dir(parts[0]))
		prefix := fmt.Sprintf("%s/", parts[1])
		hs.router.Handle(prefix, staticMiddleWare(prefix, http.StripPrefix(parts[1], fs)))
	}

	// prometheus metrics handler
	hs.router.Handle("/metrics", promhttp.HandlerFor(
		hs.monitor.Registry,
		promhttp.HandlerOpts{
			EnableOpenMetrics: true,
			Registry:          hs.monitor.Registry,
		},
	))

	// default route to handle anything else
	hs.router.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		start := time.Now()

		var err error
		var fpmErr error
		var fpmResponse *ResponseData

		worker, cancel := context.WithCancel(context.Background())
		ctx, _ := context.WithTimeout(context.Background(), hs.config.Timeout)
		go func() {
			fpmResponse, fpmErr = hs.fpmClient.Call(request)
			cancel()
		}()

		select {
		case <-ctx.Done():
			// timeout hit - return 408 and stop processing
			hs.WriteTimeout(writer, request, fmt.Errorf("timeout"), start)
			return
		case <-worker.Done():
			// everything is fine
			// fpmResponse variable is set
		}

		if fpmErr != nil {
			hs.WriteError(writer, request, fmt.Errorf("could not call FPM: %s\n", fpmErr), start)
			return
		}

		if fpmResponse == nil {
			// should never happen
			// just to be completely sure
			hs.WriteError(writer, request, fmt.Errorf("FPM response is nil"), start)
			return
		}

		hs.accessLogger.LogFpm(request, fpmResponse)

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
			hs.logger.Errorf("could not write response body: %s\n", err)
			return
		}

		hs.monitor.HttpDurationHistogram.
			WithLabelValues(
				hs.config.App,
				TypeHttp,
				request.Method,
				fmt.Sprintf("%d", fpmResponse.Status),
				fpmResponse.Route,
			).
			Observe(time.Since(start).Seconds())
	})
}

func (hs *HttpServer) WriteError(writer http.ResponseWriter, request *http.Request, err error, start time.Time) {
	hs.logger.Errorf("server error: %s\n", err)
	writer.WriteHeader(http.StatusInternalServerError)
	_, writeError := writer.Write([]byte("Internal server error"))
	if writeError != nil {
		// should not happen
		hs.logger.Errorf("could not write response body: %s\n", err)
	}
	hs.monitor.HttpDurationHistogram.
		WithLabelValues(
			hs.config.App,
			TypeHttp,
			request.Method,
			fmt.Sprintf("%d", http.StatusInternalServerError),
			"",
		).
		Observe(time.Since(start).Seconds())
}

func (hs *HttpServer) WriteTimeout(writer http.ResponseWriter, request *http.Request, err error, start time.Time) {
	hs.logger.Infof("request timeout")
	writer.WriteHeader(http.StatusRequestTimeout)
	_, writeError := writer.Write([]byte("timeout"))
	if writeError != nil {
		// should not happen
		hs.logger.Errorf("could not write response body: %s\n", err)
	}
	hs.monitor.HttpDurationHistogram.
		WithLabelValues(
			hs.config.App,
			TypeHttp,
			request.Method,
			fmt.Sprintf("%d", http.StatusRequestTimeout),
			"",
		).
		Observe(time.Since(start).Seconds())
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
