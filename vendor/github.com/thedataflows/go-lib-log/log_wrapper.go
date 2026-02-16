package log

import (
	"fmt"

	go_logging_v1 "gopkg.in/op/go-logging.v1"
)

// Trace logs a message at TraceLevel with the given package name.
func Trace(packageName string, msg any) {
	Logger().Trace().Str(KEY_PKG, packageName).Msg(fmt.Sprint(msg))
}

// Tracef logs a formatted message at TraceLevel with the given package name.
func Tracef(packageName string, format string, msg ...any) {
	Logger().Trace().Str(KEY_PKG, packageName).Msgf(format, msg...)
}

// Debug logs a message at DebugLevel with the given package name.
func Debug(packageName string, msg any) {
	Logger().Debug().Str(KEY_PKG, packageName).Msg(fmt.Sprint(msg))
}

// Debugf logs a formatted message at DebugLevel with the given package name.
func Debugf(packageName string, format string, msg ...any) {
	Logger().Debug().Str(KEY_PKG, packageName).Msgf(format, msg...)
}

// Info logs a message at InfoLevel with the given package name.
func Info(packageName string, msg any) {
	Logger().Info().Str(KEY_PKG, packageName).Msg(fmt.Sprint(msg))
}

// Infof logs a formatted message at InfoLevel with the given package name.
func Infof(packageName string, format string, msg ...any) {
	Logger().Info().Str(KEY_PKG, packageName).Msgf(format, msg...)
}

// Warn logs a message at WarnLevel with the given package name.
func Warn(packageName string, msg any) {
	Logger().Warn().Str(KEY_PKG, packageName).Msg(fmt.Sprint(msg))
}

// Warnf logs a formatted message at WarnLevel with the given package name.
func Warnf(packageName string, format string, msg ...any) {
	Logger().Warn().Str(KEY_PKG, packageName).Msgf(format, msg...)
}

// Error logs a message at ErrorLevel with the given package name and error.
func Error(packageName string, err error, msg any) {
	Logger().Error().Str(KEY_PKG, packageName).Err(err).Msg(fmt.Sprint(msg))
}

// Errorf logs a formatted message at ErrorLevel with the given package name and error.
func Errorf(packageName string, err error, format string, msg ...any) {
	Logger().Error().Str(KEY_PKG, packageName).Err(err).Msgf(format, msg...)
}

// Fatal logs a message at FatalLevel with the given package name and error, then exits.
func Fatal(packageName string, err error, msg any) {
	Logger().Fatal().Str(KEY_PKG, packageName).Err(err).Msg(fmt.Sprint(msg))
}

// Fatalf logs a formatted message at FatalLevel with the given package name and error, then exits.
func Fatalf(packageName string, err error, format string, msg ...any) {
	Logger().Fatal().Str(KEY_PKG, packageName).Err(err).Msgf(format, msg...)
}

// Panic logs a message at PanicLevel with the given package name and error, then panics.
func Panic(packageName string, err error, msg any) {
	Logger().Panic().Str(KEY_PKG, packageName).Err(err).Msg(fmt.Sprint(msg))
}

// Panicf logs a formatted message at PanicLevel with the given package name and error, then panics.
func Panicf(packageName string, err error, format string, msg ...any) {
	Logger().Panic().Str(KEY_PKG, packageName).Err(err).Msgf(format, msg...)
}

// Print logs a message at NoLevel (effectively always printing) with the given package name.
// It explicitly sets a "level" field to "-" in the output.
func Print(packageName string, msg any) {
	Logger().WithLevel(NoLevel).Str("level", "-").Str(KEY_PKG, packageName).Msg(fmt.Sprint(msg))
}

// Printf logs a formatted message at NoLevel (effectively always printing) with the given package name.
// It explicitly sets a "level" field to "-" in the output.
func Printf(packageName string, format string, msg ...any) {
	Logger().WithLevel(NoLevel).Str("level", "-").Str(KEY_PKG, packageName).Msgf(format, msg...)
}

// go-logging compatibility
// GoLoggingV1Backend implements the logging.Backend interface to bridge
// go-logging.v1 with this log package.
type GoLoggingV1Backend struct{}

// Log implements the logging.Backend interface for go-logging.v1.
// It maps go-logging levels to the corresponding log functions in this package.
func (b *GoLoggingV1Backend) Log(level go_logging_v1.Level, calldepth int, rec *go_logging_v1.Record) error {
	switch level {
	case go_logging_v1.CRITICAL:
		Error(rec.Module, nil, rec.Message())
	case go_logging_v1.ERROR:
		Error(rec.Module, nil, rec.Message())
	case go_logging_v1.WARNING:
		Warn(rec.Module, rec.Message())
	case go_logging_v1.DEBUG:
		Debug(rec.Module, rec.Message())
	default:
		Info(rec.Module, rec.Message())
	}

	return nil
}
