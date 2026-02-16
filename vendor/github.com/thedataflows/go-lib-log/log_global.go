// Package log provides global logger functionality and state management.
// This file contains all global logger objects and functions that were moved
// from log.go to improve code organization and maintainability.
package log

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
)

var (
	// globalConfigMutex protects configuration changes to globalLoggerBuilder
	// We use a separate mutex for config changes since they're rare
	globalConfigMutex sync.Mutex
	// globalLoggerBuilder is the builder instance used for the global Logger.
	// This preserves user configuration when functions like SetGlobalLoggerLogFormat are called.
	globalLoggerBuilder = NewLoggerBuilder()
	// globalLogger is the global instance of the CustomLogger using atomic pointer for lock-free reads
	globalLogger atomic.Pointer[CustomLogger]
)

// init initializes the global logger with the default configuration
func init() {
	zerolog.ErrorStackMarshaler = StackMarshaller
	globalLogger.Store(globalLoggerBuilder.Build())
}

// Flush ensures all buffered messages in the global Logger are written to the output.
// This is useful when you want to force immediate output without closing the logger.
func Flush() {
	logger := globalLogger.Load()
	if logger != nil {
		logger.Flush()
	}
}

// SetGlobalLoggerLogLevel sets the global log level for the default Logger.
// If the provided level string is empty, it defaults to InfoLevel.
// It returns an error if the level string is invalid.
// This preserves all other configuration settings of the global logger.
func SetGlobalLoggerLogLevel(level string) error {
	if len(level) == 0 {
		level = InfoLevel.String()
	}
	parsedLevel, err := ParseLevel(level)
	if err != nil {
		return err
	}

	globalConfigMutex.Lock()
	defer globalConfigMutex.Unlock()

	// Close the current logger to clean up resources
	if currentLogger := globalLogger.Load(); currentLogger != nil {
		currentLogger.Close()
	}

	// Update the global builder's level setting and rebuild
	globalLoggerBuilder.logLevel = parsedLevel

	// Rebuild the logger with preserved settings
	globalLogger.Store(globalLoggerBuilder.Build())

	return nil
}

// SetGlobalLoggerBuilder replaces the global Logger with a new one built from the provided builder.
// This allows users to configure the global logger with custom settings while ensuring
// that subsequent calls to SetGlobalLoggerLogFormat preserve those settings.
func SetGlobalLoggerBuilder(builder *LoggerBuilder) {
	globalConfigMutex.Lock()
	defer globalConfigMutex.Unlock()

	// Close the current global logger if it exists
	if currentLogger := globalLogger.Load(); currentLogger != nil {
		currentLogger.Close()
	}

	// Update the global builder and create new logger
	globalLoggerBuilder = builder
	globalLogger.Store(globalLoggerBuilder.Build())
}

// GlobalLoggerBuilder returns the global logger builder instance.
// This allows users to access the current configuration and modify it if needed.
func GlobalLoggerBuilder() *LoggerBuilder {
	globalConfigMutex.Lock()
	defer globalConfigMutex.Unlock()
	return globalLoggerBuilder
}

// SetGlobalLogger replaces the global Logger with a new one.
// This allows users to configure the global logger with custom settings while ensuring
// that subsequent calls to SetGlobalLoggerLogFormat preserve those settings.
func SetGlobalLogger(logger *CustomLogger) {
	globalConfigMutex.Lock()
	defer globalConfigMutex.Unlock()

	// Close the current global logger if it exists
	if currentLogger := globalLogger.Load(); currentLogger != nil {
		currentLogger.Close()
	}
	globalLogger.Store(logger)
}

// Logger returns the global Logger instance.
func Logger() *CustomLogger {
	logger := globalLogger.Load()
	if logger != nil {
		return logger
	}

	// If logger is nil (shouldn't happen normally, but might in tests),
	// create a new one using the globalLoggerBuilder
	globalConfigMutex.Lock()
	defer globalConfigMutex.Unlock()

	// Double-check after acquiring lock
	if logger = globalLogger.Load(); logger != nil {
		return logger
	}

	if globalLoggerBuilder == nil {
		globalLoggerBuilder = NewLoggerBuilder()
	}
	logger = globalLoggerBuilder.Build()
	globalLogger.Store(logger)
	return logger
}

// Close closes the global Logger instance, ensuring cleanup of its resources.
// This should be called when the application is shutting down to flush any buffered logs.
func Close() {
	globalConfigMutex.Lock()
	defer globalConfigMutex.Unlock()

	if currentLogger := globalLogger.Load(); currentLogger != nil {
		currentLogger.Close()
	}
}

// StackMarshaller is a basic marshaller that outputs the current stack trace to stderr.
func StackMarshaller(_ error) interface{} {
	var pcs [32]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		_, _ = fmt.Fprintf(os.Stderr, "%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
		if !more {
			break
		}
	}
	return nil
}
