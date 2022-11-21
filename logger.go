package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/exp/slog"
)

type leveledLogger = interface {
	Error(msg string, err error, keysAndValues ...interface{})
	Info(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
}

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

func Debugf(l leveledLogger, msg string, args ...interface{}) {
	l.Debug(fmt.Sprintf(msg, args...))
}

func LogFatal(l leveledLogger, msg string, err error, args ...interface{}) {
	l.Error("", err, args...)
	os.Exit(2)
}

type retryableHTTPClientloggerWrapper struct {
	*slog.Logger
}

func (l retryableHTTPClientloggerWrapper) Error(msg string, keysAndValues ...interface{}) {
	l.Logger.Error(msg, nil, keysAndValues...)
}

func (l retryableHTTPClientloggerWrapper) Debug(msg string, keysAndValues ...interface{}) {
	// ignore very verbose debug output from retryableHTTPClientloggerWrapper
}

// wrapLoggerForRetryableHTTPClient a logger so that it implements an interface required by retryableHTTPClient
func wrapLoggerForRetryableHTTPClient(logger *slog.Logger) retryableHTTPClientloggerWrapper {
	// ignore debug messages from this retry client.
	l := slog.New(logger.Handler())
	return retryableHTTPClientloggerWrapper{l}
}
