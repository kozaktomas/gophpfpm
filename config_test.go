package main

import (
	"io"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

func TestLoadConfig_Defaults(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)

	// Define flags with defaults
	flags.Int(ParamPort, 8080, "")
	flags.String(ParamSocket, "/tmp/php-fpm.sock", "")
	flags.String(ParamIndex, "/var/www/index.php", "")
	flags.String(ParamApp, "php-app", "")
	flags.StringArray(ParamStaticFolders, []string{}, "")
	flags.Int(FpmPoolSize, 32, "")
	flags.Duration("timeout", 30*time.Second, "")
	flags.Bool(AccessLog, false, "")
	flags.Bool(ParamVerbose, false, "")

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	config, err := LoadConfig(flags, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Port != 8080 {
		t.Errorf("Port = %d, want 8080", config.Port)
	}

	if config.App != "php-app" {
		t.Errorf("App = %q, want %q", config.App, "php-app")
	}

	if config.FpmPoolSize != 32 {
		t.Errorf("FpmPoolSize = %d, want 32", config.FpmPoolSize)
	}

	if config.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", config.Timeout)
	}

	if config.AccessLog != false {
		t.Errorf("AccessLog = %v, want false", config.AccessLog)
	}

	if config.Verbose != false {
		t.Errorf("Verbose = %v, want false", config.Verbose)
	}
}

func TestLoadConfig_CustomValues(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)

	// Define flags
	flags.Int(ParamPort, 8080, "")
	flags.String(ParamSocket, "", "")
	flags.String(ParamIndex, "", "")
	flags.String(ParamApp, "php-app", "")
	flags.StringArray(ParamStaticFolders, []string{}, "")
	flags.Int(FpmPoolSize, 32, "")
	flags.Duration("timeout", 30*time.Second, "")
	flags.Bool(AccessLog, false, "")
	flags.Bool(ParamVerbose, false, "")

	// Set custom values
	_ = flags.Set(ParamPort, "9000")
	_ = flags.Set(ParamSocket, "/custom/socket.sock")
	_ = flags.Set(ParamIndex, "/app/public/index.php")
	_ = flags.Set(ParamApp, "my-app")
	_ = flags.Set(FpmPoolSize, "64")
	_ = flags.Set("timeout", "1m")
	_ = flags.Set(AccessLog, "true")
	_ = flags.Set(ParamVerbose, "true")

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	config, err := LoadConfig(flags, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Port != 9000 {
		t.Errorf("Port = %d, want 9000", config.Port)
	}

	if config.Socket != "/custom/socket.sock" {
		t.Errorf("Socket = %q, want %q", config.Socket, "/custom/socket.sock")
	}

	if config.IndexFile != "/app/public/index.php" {
		t.Errorf("IndexFile = %q, want %q", config.IndexFile, "/app/public/index.php")
	}

	if config.App != "my-app" {
		t.Errorf("App = %q, want %q", config.App, "my-app")
	}

	if config.FpmPoolSize != 64 {
		t.Errorf("FpmPoolSize = %d, want 64", config.FpmPoolSize)
	}

	if config.Timeout != 1*time.Minute {
		t.Errorf("Timeout = %v, want 1m", config.Timeout)
	}

	if config.AccessLog != true {
		t.Errorf("AccessLog = %v, want true", config.AccessLog)
	}

	if config.Verbose != true {
		t.Errorf("Verbose = %v, want true", config.Verbose)
	}
}

func TestLoadConfig_StaticFolders(t *testing.T) {
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)

	flags.Int(ParamPort, 8080, "")
	flags.String(ParamSocket, "", "")
	flags.String(ParamIndex, "", "")
	flags.String(ParamApp, "php-app", "")
	flags.StringArray(ParamStaticFolders, []string{}, "")
	flags.Int(FpmPoolSize, 32, "")
	flags.Duration("timeout", 30*time.Second, "")
	flags.Bool(AccessLog, false, "")
	flags.Bool(ParamVerbose, false, "")

	_ = flags.Set(ParamStaticFolders, "/var/www/static:/static")
	_ = flags.Set(ParamStaticFolders, "/var/www/assets:/assets")

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	config, err := LoadConfig(flags, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(config.StaticFolders) != 2 {
		t.Errorf("StaticFolders length = %d, want 2", len(config.StaticFolders))
	}

	if config.StaticFolders[0] != "/var/www/static:/static" {
		t.Errorf("StaticFolders[0] = %q, want %q", config.StaticFolders[0], "/var/www/static:/static")
	}
}

func TestIgnoreError(t *testing.T) {
	// Test with string
	strResult := ignoreError("hello", nil)
	if strResult != "hello" {
		t.Errorf("ignoreError string = %q, want %q", strResult, "hello")
	}

	// Test with int
	intResult := ignoreError(42, nil)
	if intResult != 42 {
		t.Errorf("ignoreError int = %d, want 42", intResult)
	}

	// Test with bool
	boolResult := ignoreError(true, nil)
	if boolResult != true {
		t.Errorf("ignoreError bool = %v, want true", boolResult)
	}

	// Test that error is actually ignored
	strWithErr := ignoreError("value", io.EOF)
	if strWithErr != "value" {
		t.Errorf("ignoreError with error = %q, want %q", strWithErr, "value")
	}
}
