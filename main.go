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
		Use:   "gofpmproxy",
		Short: "Super fast HTTP proxy server for PHP FPM",
		Long:  `Long description`,
		Run: func(cmd *cobra.Command, args []string) {
			config := LoadConfig(cmd.PersistentFlags(), logger)
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
