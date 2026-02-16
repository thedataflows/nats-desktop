package log

import (
	"context"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

const (
	// ENV_LOG_BUFFER_SIZE is the environment variable for the log buffer size.
	ENV_LOG_BUFFER_SIZE = "LOG_BUFFER_SIZE"
	// ENV_LOG_RATE_LIMIT is the environment variable for the log rate limit.
	ENV_LOG_RATE_LIMIT = "LOG_RATE_LIMIT" // messages per second
	// ENV_LOG_RATE_BURST is the environment variable for the log rate burst size.
	ENV_LOG_RATE_BURST = "LOG_RATE_BURST" // burst size
	// ENV_LOG_DROP_REPORT_INTERVAL is the environment variable for the log drop report interval.
	ENV_LOG_DROP_REPORT_INTERVAL = "LOG_DROP_REPORT_INTERVAL" // seconds between drop reports
	// ENV_LOG_GROUP_WINDOW is the environment variable for the log event grouping window.
	ENV_LOG_GROUP_WINDOW = "LOG_GROUP_WINDOW" // seconds for event grouping window
	// ENV_LOG_BUFFERING_DISABLED is the environment variable to disable buffering.
	ENV_LOG_BUFFERING_DISABLED = "LOG_BUFFERING_DISABLED" // set to "true" to disable buffering
	// DEFAULT_BUFFER_SIZE is the default buffer size for the logger.
	DEFAULT_BUFFER_SIZE = 100000 // High throughput: 100K buffer
	// DEFAULT_RATE_LIMIT is the default rate limit for the logger in messages per second.
	DEFAULT_RATE_LIMIT = 50000 // High throughput: 50K msgs/sec
	// DEFAULT_RATE_BURST is the default rate burst for the logger.
	DEFAULT_RATE_BURST = 10000 // High throughput: 10K burst
	// DEFAULT_DROP_REPORT_INTERVAL is the default interval in seconds for reporting dropped messages.
	DEFAULT_DROP_REPORT_INTERVAL = 10 // Report drops every 10 seconds
	// DEFAULT_GROUP_WINDOW is the default window in seconds for grouping similar events.
	DEFAULT_GROUP_WINDOW = 1 // Group events over 1 second window
)

// LoggerBuilder provides a fluent interface for configuring and building CustomLogger instances.
type LoggerBuilder struct {
	output             io.Writer
	bufferSize         int
	rateLimit          int
	rateBurst          int
	groupWindow        time.Duration
	dropReportInterval time.Duration
	bufferingDisabled  bool
	logLevel           zerolog.Level
	logFormat          LogFormat
}

// getBufferingDisabled checks if buffering is explicitly disabled via environment variable.
func getBufferingDisabled() bool {
	v := strings.ToLower(os.Getenv(ENV_LOG_BUFFERING_DISABLED))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}

// NewLoggerBuilder creates a new LoggerBuilder with default configuration.
// This is the only constructor function - all other configurations are done through the builder pattern.
func NewLoggerBuilder() *LoggerBuilder {
	logFormat, _ := ParseLogFormat(os.Getenv(ENV_LOG_FORMAT))
	logLevel, _ := ParseLevel(os.Getenv(ENV_LOG_LEVEL))
	return &LoggerBuilder{
		bufferSize:         getEnvInt(ENV_LOG_BUFFER_SIZE, DEFAULT_BUFFER_SIZE),
		rateLimit:          getEnvInt(ENV_LOG_RATE_LIMIT, DEFAULT_RATE_LIMIT),
		rateBurst:          getEnvInt(ENV_LOG_RATE_BURST, DEFAULT_RATE_BURST),
		groupWindow:        time.Duration(getEnvInt(ENV_LOG_GROUP_WINDOW, DEFAULT_GROUP_WINDOW)) * time.Second,
		dropReportInterval: time.Duration(getEnvInt(ENV_LOG_DROP_REPORT_INTERVAL, DEFAULT_DROP_REPORT_INTERVAL)) * time.Second,
		bufferingDisabled:  getBufferingDisabled(),
		logLevel:           logLevel,
		logFormat:          logFormat,
	}
}

// WithOutput sets the output writer for the logger.
func (lb *LoggerBuilder) WithOutput(output io.Writer) *LoggerBuilder {
	lb.output = output
	return lb
}

// WithBufferSize sets the buffer size for the logger.
func (lb *LoggerBuilder) WithBufferSize(size int) *LoggerBuilder {
	lb.bufferSize = size
	return lb
}

// WithRateLimit sets the rate limit (messages per second) for the logger.
func (lb *LoggerBuilder) WithRateLimit(limit int) *LoggerBuilder {
	lb.rateLimit = limit
	return lb
}

// WithRateBurst sets the rate burst size for the logger.
func (lb *LoggerBuilder) WithRateBurst(burst int) *LoggerBuilder {
	lb.rateBurst = burst
	return lb
}

// WithGroupWindow sets the event grouping window duration.
// Use 0 for default from environment, negative values to disable grouping.
func (lb *LoggerBuilder) WithGroupWindow(window time.Duration) *LoggerBuilder {
	lb.groupWindow = window
	return lb
}

// WithDropReportInterval sets the interval for reporting dropped messages.
// Use 0 for default from environment, negative values to disable drop reporting.
func (lb *LoggerBuilder) WithDropReportInterval(interval time.Duration) *LoggerBuilder {
	lb.dropReportInterval = interval
	return lb
}

// WithoutBuffering disables buffering for the logger.
func (lb *LoggerBuilder) WithoutBuffering() *LoggerBuilder {
	lb.bufferingDisabled = true
	return lb
}

// WithLogLevel sets the log level.
func (lb *LoggerBuilder) WithLogLevel(level zerolog.Level) *LoggerBuilder {
	lb.logLevel = level
	return lb
}

// WithLogLevelString sets the log level from a string.
func (lb *LoggerBuilder) WithLogLevelString(level string) *LoggerBuilder {
	if parsedLevel, err := ParseLevel(level); err == nil {
		lb.logLevel = parsedLevel
	}
	return lb
}

// WithLogFormat sets the log format.
func (lb *LoggerBuilder) WithLogFormat(format LogFormat) *LoggerBuilder {
	lb.logFormat = format
	return lb
}

// Build creates and returns the configured CustomLogger instance.
func (lb *LoggerBuilder) Build() *CustomLogger {
	// Determine output writer
	output := lb.output
	if output == nil {
		switch lb.logFormat {
		case LOG_FORMAT_JSON:
			output = os.Stderr
		default:
			output = zerolog.ConsoleWriter{
				Out:        PreferredWriter(),
				TimeFormat: time.RFC3339,
			}
		}
	}

	// Create the buffered rate limited writer
	bufferedWriter := newBufferedRateLimitedWriter(
		output,
		lb.bufferSize,
		lb.rateLimit,
		lb.rateBurst,
		lb.groupWindow,
		lb.dropReportInterval,
		lb.bufferingDisabled,
	)

	// Create the zerolog logger
	zl := zerolog.New(bufferedWriter).
		With().
		Timestamp().
		Logger().
		Level(lb.getLogLevel())

	return &CustomLogger{
		Logger:     zl,
		writer:     bufferedWriter,
		bufferSize: lb.bufferSize,
		logFormat:  lb.logFormat,
	}
}

// getLogLevel returns the log level, using builder value or environment default.
func (lb *LoggerBuilder) getLogLevel() zerolog.Level {
	if lb.logLevel != NoLevel {
		return lb.logLevel
	}
	return InfoLevel
}

// getEnvInt parses an integer from environment variable with fallback to default.
func getEnvInt(envVar string, defaultValue int) int {
	if str := os.Getenv(envVar); str != "" {
		if val, err := strconv.Atoi(str); err == nil && val > 0 {
			return val
		}
	}
	return defaultValue
}

// newBufferedRateLimitedWriter is the internal constructor that handles all options.
func newBufferedRateLimitedWriter(target io.Writer, bufferSize, rateLimit, rateBurst int, groupWindow, dropReportInterval time.Duration, bufferingDisabled bool) *BufferedRateLimitedWriter {
	// Get drop report interval from parameter or environment fallback
	if dropReportInterval <= 0 {
		dropReportInterval = time.Duration(
			getEnvInt(ENV_LOG_DROP_REPORT_INTERVAL, DEFAULT_DROP_REPORT_INTERVAL),
		) * time.Second
	}

	// Get group window from environment if not explicitly provided (groupWindow == 0)
	if groupWindow == 0 {
		groupWindow = time.Duration(getEnvInt(ENV_LOG_GROUP_WINDOW, DEFAULT_GROUP_WINDOW)) * time.Second
	}
	// If groupWindow is negative, it means grouping is explicitly disabled
	if groupWindow < 0 {
		groupWindow = 0
	}

	w := &BufferedRateLimitedWriter{
		target:             target,
		limiter:            rate.NewLimiter(rate.Limit(rateLimit), rateBurst),
		buffer:             make(chan []byte, bufferSize),
		closeSignal:        make(chan struct{}), // Initialize closeSignal
		dropReportInterval: dropReportInterval,
		grouper:            newEventGrouper(groupWindow, target),
		bufferingDisabled:  bufferingDisabled,
	}

	// Create cancellable context for rate limiter
	w.ctx, w.cancelFunc = context.WithCancel(context.Background())

	// Pre-compute static parts of drop report message for performance
	w.intervalSecondsStr = strconv.FormatFloat(w.dropReportInterval.Seconds(), 'f', 0, 64)
	w.dropReportPrefix = []byte(`{"time":"`)
	w.dropReportSuffix = []byte(`","level":"warn","message":"Log messages dropped due to backpressure","dropped_count":`)

	return w
}
