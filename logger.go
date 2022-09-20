package main

import (
	"github.com/go-kit/log"
	"os"
)

func newLogger(level logLevel) leveledLogger {
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	return leveledLogger{
		logger: logger,
		level:  level,
	}
}

type leveledLogger struct {
	logger log.Logger
	level  logLevel
}

type logLevel int

const logLevelDebug logLevel = 0
const logLevelInfo logLevel = 1
const logLevelWarn logLevel = 2
const logLevelError logLevel = 3

func (l leveledLogger) Err(err error, keysAndValues ...interface{}) {
	if l.level > logLevelError {
		return
	}
	k := append(keysAndValues, "message", err.Error(), "level", "ERROR")
	l.logger.Log(k...)
}

func (l leveledLogger) Error(msg string, keysAndValues ...interface{}) {
	if l.level > logLevelError {
		return
	}
	k := append(keysAndValues, "message", msg, "level", "ERROR")
	l.logger.Log(k...)
}

func (l leveledLogger) Warn(msg string, keysAndValues ...interface{}) {
	if l.level > logLevelWarn {
		return
	}
	k := append(keysAndValues, "message", msg, "level", "WAR")
	l.logger.Log(k...)

}

func (l leveledLogger) Info(msg string, keysAndValues ...interface{}) {
	if l.level > logLevelInfo {
		return
	}
	k := append(keysAndValues, "message", msg, "level", "INFO")
	l.logger.Log(k...)
}

func (l leveledLogger) Debug(msg string, keysAndValues ...interface{}) {
	if l.level > logLevelDebug {
		return
	}
	k := append(keysAndValues, "message", msg, "level", "DEBUG")
	l.logger.Log(k...)
}
