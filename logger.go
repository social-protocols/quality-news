package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/exp/slog"
)

func newLogger(levelString, formatString string) *slog.Logger {
	if levelString == "" {
		levelString = "DEBUG"
	}

	logLevels := map[string]slog.Leveler{
		"DEBUG": slog.DebugLevel,
		"INFO":  slog.InfoLevel,
		"WARN":  slog.WarnLevel,
		"ERROR": slog.ErrorLevel,
	}

	l, ok := logLevels[strings.ToUpper(levelString)]
	if !ok {
		panic("Unrecognized log level: " + levelString)
	}

	var lh slog.Handler

	if strings.ToUpper(formatString) == "JSON" {
		lh = slog.HandlerOptions{Level: l}.NewJSONHandler(os.Stdout)
	} else {
		lh = slog.HandlerOptions{Level: l}.NewTextHandler(os.Stdout)
	}

	logger := slog.New(lh)
	slog.SetDefault(logger)
	return logger
}

func LogErrorf(logger *slog.Logger, msg string, args ...interface{}) {
	logger.Error(fmt.Sprintf(msg, args...), nil)
}

func Debugf(logger *slog.Logger, msg string, args ...interface{}) {
	logger.Debug(fmt.Sprintf(msg, args...))
}

func LogFatal(logger *slog.Logger, msg string, err error, args ...interface{}) {
	if len(args) > 0 {
		logger.Error(msg, err, args...)
	} else {
		logger.Error(msg, err)
	}
	os.Exit(2)
}

type retryableHTTPClientloggerWrapper struct {
	*slog.Logger
}

func (l retryableHTTPClientloggerWrapper) Error(msg string, keysAndValues ...interface{}) {
	l.Logger.Error("retryableHTTPClient: "+msg, nil, keysAndValues...)
}

func (l retryableHTTPClientloggerWrapper) Debug(msg string, keysAndValues ...interface{}) {
	// ignore very verbose debug output from retryableHTTPClientloggerWrapper
}

// wrapLoggerForRetryableHTTPClient wraps a logger so that it implements an interface required by retryableHTTPClient
func wrapLoggerForRetryableHTTPClient(logger *slog.Logger) retryableHTTPClientloggerWrapper {
	// ignore debug messages from this retry client.
	l := slog.New(logger.Handler())
	return retryableHTTPClientloggerWrapper{l}
}
