package main

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	protectedHeadersInbound = map[string]bool{
		"content-type":   true,
		"content-length": true,
	}

	protectedHeadersOutbound = map[string]bool{
		"x-powered-by": true,
		"x-app-route":  true,
	}
)

func main() {
	logger := log.New()
	logger.SetFormatter(&log.JSONFormatter{})
	logger.SetLevel(log.DebugLevel)

	rootCmd := &cobra.Command{
		Use:   "gophpfpm",
		Short: "Super fast HTTP proxy server for PHP FPM",
		Long:  `Web server for PHP written in Go. It's compatible with PHP-FPM communicating via FastCGI protocol using unix socket.`,
		Run: func(cmd *cobra.Command, args []string) {
			config, err := LoadConfig(cmd.PersistentFlags(), logger)
			if err != nil {
				logger.Fatalf("could not load config: %s", err)
			}
			logger.SetLevel(log.InfoLevel)
			if config.Verbose {
				logger.SetLevel(log.DebugLevel)
			}

			fCgiClient, err := NewFCgiClient(config, logger)
			if err != nil {
				logger.Fatalf("could not create FPM client: %s", err)
			}

			accessLogger := NewAccessLogger(config, logger)
			monitor := NewMonitor(logger)
			fpmClient := NewFpmClient(fCgiClient, config, monitor, logger)
			svr := NewHttpServer(config, fpmClient, accessLogger, monitor, logger)
			svr.PrepareServer()

			config.LogConfig()
			svr.StartServer()
		},
	}

	DefineParams(rootCmd)
	if err := rootCmd.Execute(); err != nil {
		logger.Fatalf("could not run root command")
	}
	return

}
