package main

import (
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	ParamPort          = "port"
	ParamSocket        = "socket"
	ParamIndex         = "index-file"
	ParamApp           = "app"
	ParamStaticFolders = "static-folder"
	FpmPoolSize        = "fpm-pool-size"
	Timeout            = "timeout"
	AccessLog          = "access-log"
	ParamVerbose       = "verbose"
)

type Config struct {
	Port          int           // port to listen on
	Socket        string        // path to php-fpm socket
	IndexFile     string        // index.php file path
	App           string        // application name
	StaticFolders []string      // list of static folders
	FpmPoolSize   int           // number of connections to php-fpm
	Timeout       time.Duration // timeout for connection
	AccessLog     bool          // enable access logging
	Verbose       bool          // print debug output

	logger *log.Logger
}

func DefineParams(cmd *cobra.Command) {
	cmd.PersistentFlags().IntP(ParamPort, "p", 8080, "Go FPM proxy port")
	cmd.PersistentFlags().StringP(ParamSocket, "s", "", "Path to PHP-FPM UNIX Socket")
	cmd.PersistentFlags().StringP(ParamIndex, "i", "", "Path to index.php script in the PHP-FPM container")
	cmd.PersistentFlags().String(ParamApp, "php-app", "Application name")
	cmd.PersistentFlags().StringArrayP(ParamStaticFolders, "f", []string{}, fmt.Sprintf("Static folder in format %q", "/home/path/to/folder:/endpoint/prefix"))
	cmd.PersistentFlags().Int(FpmPoolSize, 32, "Size of the FPM pool")
	cmd.PersistentFlags().Duration("timeout", 30*time.Second, "Timeout for connection [10s, 30s, 1m]")
	cmd.PersistentFlags().Bool(AccessLog, false, "Enable access logging")
	cmd.PersistentFlags().BoolP(ParamVerbose, "v", false, "Print debug output")

	_ = cmd.MarkPersistentFlagRequired(ParamSocket)
	_ = cmd.MarkPersistentFlagRequired(ParamIndex)
}

func LoadConfig(set *pflag.FlagSet, logger *log.Logger) (*Config, error) {
	timeout, err := set.GetDuration("timeout")
	if err != nil {
		return nil, fmt.Errorf("could not load %q: %s", Timeout, err)
	}

	return &Config{
		Port:          ignoreError(set.GetInt(ParamPort)),
		Socket:        ignoreError(set.GetString(ParamSocket)),
		IndexFile:     ignoreError(set.GetString(ParamIndex)),
		App:           ignoreError(set.GetString(ParamApp)),
		StaticFolders: ignoreError(set.GetStringArray(ParamStaticFolders)),
		FpmPoolSize:   ignoreError(set.GetInt(FpmPoolSize)),
		Timeout:       timeout,
		AccessLog:     ignoreError(set.GetBool(AccessLog)),
		Verbose:       ignoreError(set.GetBool(ParamVerbose)),

		logger: logger,
	}, nil
}

func (c *Config) LogConfig() {
	c.logger.Infof("[CONFIG] Port: %d", c.Port)
	c.logger.Infof("[CONFIG] Socket: %s", c.Socket)
	c.logger.Infof("[CONFIG] Index file %s", c.IndexFile)
	c.logger.Infof("[CONFIG] App: %s", c.App)
	c.logger.Infof("[CONFIG] Static folders: %s", strings.Join(c.StaticFolders, ","))
	c.logger.Infof("[CONFIG] Timeout: %s", c.Timeout)
	c.logger.Infof("[CONFIG] FPM pool size: %d", c.FpmPoolSize)
	c.logger.Infof("[CONFIG] Access logging: %t", c.AccessLog)
	c.logger.Infof("[CONFIG] Verbose: %t", c.Verbose)
}

func ignoreError[K string | bool | int | []string](value K, _ error) K {
	return value
}
