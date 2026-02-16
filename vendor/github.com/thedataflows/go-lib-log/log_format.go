package log

import "fmt"

// LogFormat defines the available log output formats.
type LogFormat int

const (
	// LOG_FORMAT_CONSOLE represents a human-readable console log format.
	LOG_FORMAT_CONSOLE LogFormat = iota
	// LOG_FORMAT_JSON represents a machine-readable JSON log format.
	LOG_FORMAT_JSON
	_logFormatCount // sentinel for count
)

// AllLogFormats is a list of all supported LogFormat values.
var AllLogFormats = [_logFormatCount]LogFormat{
	LOG_FORMAT_CONSOLE,
	LOG_FORMAT_JSON,
}

// String implements fmt.Stringer interface
func (f LogFormat) String() string {
	switch f {
	case LOG_FORMAT_CONSOLE:
		return "console"
	case LOG_FORMAT_JSON:
		return "json"
	default:
		return "unknown"
	}
}

// ParseLogFormat converts a string representation of a log format into a LogFormat type.
// It returns LOG_FORMAT_CONSOLE and an error for any other invalid input.
func ParseLogFormat(s string) (LogFormat, error) {
	switch s {
	case "", "console": // empty defaults to console
		return LOG_FORMAT_CONSOLE, nil
	case "json":
		return LOG_FORMAT_JSON, nil
	default:
		return LOG_FORMAT_CONSOLE, fmt.Errorf("invalid log format '%s'. Supported formats: %v", s, AllLogFormatsStrings())
	}
}

// AllLogFormatsStrings returns a slice of strings representing all supported log formats.
func AllLogFormatsStrings() []string {
	formats := make([]string, len(AllLogFormats))
	for i, format := range AllLogFormats {
		formats[i] = format.String()
	}
	return formats
}

// IsValidLogFormat checks if a given string is a valid log format.
func IsValidLogFormat(s string) bool {
	for _, format := range AllLogFormats {
		if format.String() == s {
			return true
		}
	}
	return false
}

// SetGlobalLoggerLogFormat sets the log format for the global Logger.
// It parses the provided logFormat string and updates the Logger's
// underlying writer to either a console writer or a JSON writer.
// This preserves all other configuration settings of the global logger.
// Returns an error if the logFormat string is invalid.
func SetGlobalLoggerLogFormat(logFormat string) error {
	format, err := ParseLogFormat(logFormat)
	if err != nil {
		return err
	}

	globalConfigMutex.Lock()
	defer globalConfigMutex.Unlock()

	// Close the current logger's writer to clean up resources
	if currentLogger := globalLogger.Load(); currentLogger != nil {
		currentLogger.Close()
	}

	// Update the global builder's format setting and rebuild
	globalLoggerBuilder.logFormat = format
	globalLogger.Store(globalLoggerBuilder.Build())

	return nil
}
