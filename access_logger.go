package main

import (
	"github.com/sirupsen/logrus"
	"net/http"
)

type AccessLogger struct {
	config *Config
	logger *logrus.Logger
}

func NewAccessLogger(config *Config, logger *logrus.Logger) *AccessLogger {
	return &AccessLogger{
		config: config,
		logger: logger,
	}
}

func (accessLogger *AccessLogger) LogFpm(request *http.Request, response *ResponseData) {
	if !accessLogger.config.AccessLog {
		return // do not log access logs
	}

	if request == nil {
		accessLogger.logger.Errorf("could not log FPM request because request is nil")
		return
	}

	if request.URL == nil {
		accessLogger.logger.Errorf("could not log FPM request because request URL is nil")
		return
	}

	if response == nil {
		accessLogger.logger.Errorf("could not log FPM request because response is nil")
		return
	}

	accessLogger.logger.WithFields(logrus.Fields{
		"method": request.Method,
		"query":  request.URL.Query(),
		"status": response.Status,
		"route":  response.Route,
		"size":   len(response.Body),
	}).Info("access")
}
