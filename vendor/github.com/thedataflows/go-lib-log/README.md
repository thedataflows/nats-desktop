# go-lib-log

A simple, high-performance, and configurable logging library for Go, built on [zerolog](https://github.com/rs/zerolog).

This library aims to provide a consistent logging interface with advanced features like rate limiting, buffering, and structured logging, suitable for high-throughput applications.

## Overview

`go-lib-log` is a high-performance logging library for Go that wraps `zerolog` with additional production-ready features. It provides a clean builder pattern API for creating customized loggers with advanced capabilities like event grouping, buffering, and rate limiting.

## Features

### Core Logging Features

* **Multiple Log Levels**: Supports Trace, Debug, Info, Warn, Error, Fatal, and Panic levels
* **Structured Logging**: Output in JSON or human-readable console format
* **Builder Pattern API**: Modern, chainable configuration using `NewLoggerBuilder()`
* **Environment Variable Configuration**: Easy setup in different environments without code changes
* **Compatibility Layer**: Maintains compatibility with `go-logging.v1`

### Performance Features

* **High-Performance Buffering**: Asynchronous processing with configurable buffer sizes
* **Rate Limiting**: Prevents log flooding with configurable limits and burst handling
* **Zero-Allocation JSON**: Leverages `zerolog`'s efficient JSON marshaling
* **Reduced Lock Contention**: Optimized internal locking for concurrent applications

### Advanced Features

* **Event Grouping**: Automatically groups identical log messages within configurable time windows to reduce noise and improve readability
* **Backpressure Handling**: Automatic reporting of dropped log messages due to buffer overflow
* **Flexible Buffering Control**: Choose between buffered (high performance) or unbuffered (immediate write) modes
* **Custom Output Destinations**: Support for custom writers and output formats

### Configuration Features

* **Programmatic Configuration**: Full control via builder pattern methods
* **Environment Variable Support**: Configure all features via environment variables
* **Runtime Flexibility**: Mix and match features as needed (grouping, buffering, rate limiting)

## Performance

`go-lib-log` is designed for high performance, leveraging `zerolog`\'s efficient JSON marshaling and introducing its own optimizations for buffering and rate limiting.

In benchmarks, `go-lib-log` with its `BufferedRateLimitedWriter` (which includes features like asynchronous processing, rate limiting, and drop reporting) demonstrates significantly better performance compared to the standard library\'s `log` package, especially in high-throughput scenarios.

Here\'s a comparative overview of approximate performance for a simple logging operation (logging a short string and an integer):

| Feature / Logger                                       | `go-lib-log` (Buffered) | Standard Library `log` | `zerolog` (Direct) |
| :----------------------------------------------------- | :---------------------- | :--------------------- | :----------------- |
| **Approx. ns/op**                                      | ~33 ns/op               | ~350+ ns/op            | ~30 ns/op          |
| **Event Grouping Overhead**                            | ~2-7% (when enabled)    | N/A                    | N/A                |
| **Asynchronous Processing**                            | Yes (Buffered)          | No (Synchronous)       | No (Synchronous)   |
| **Structured Logging (JSON)**                          | Yes (via zerolog)       | Manual / Verbose       | Yes (Core)         |
| **Rate Limiting**                                      | Yes                     | No                     | No                 |
| **Buffering**                                          | Yes                     | No                     | No                 |
| **Dropped Message Reporting**                          | Yes                     | No                     | No                 |
| **Zero-Allocation JSON (underlying)**                  | Yes (via zerolog)       | No                     | Yes (Core)         |

**Notes:**

* Performance numbers can vary based on the specific benchmark, hardware, and logging configuration (e.g., output destination, log format).
* Event grouping adds minimal overhead (~2-7%) but provides significant benefits in reducing log noise and storage costs.
* The Standard Library `log` figures are for attempts to achieve similar structured-like output; basic `log.Println` would be faster but unstructured.
* `zerolog` (Direct) serves as a baseline for the underlying logging engine without the additional features of `go-lib-log`\'s writer.

The key takeaway is that `go-lib-log` aims to provide advanced logging features (like buffering and rate limiting) with a minimal performance overhead compared to raw `zerolog`, and a substantial improvement over less optimized or more feature-heavy logging solutions, including the standard library logger when structured output and asynchronous behavior are desired.

The performance benefits are primarily due to:

* **Asynchronous Processing**: Log messages are written to an in-memory buffer and processed by a separate goroutine, minimizing blocking in the application\'s hot paths.
* **Efficient Serialization**: `zerolog`\'s focus on zero-allocation JSON marshaling.
* **Reduced Lock Contention**: Recent optimizations have further reduced internal lock contention, particularly in the `BufferedRateLimitedWriter`.

When comparing, ensure that the standard library logger is configured to produce a similar output format (e.g., JSON-like or structured text) and consider its synchronous nature by default, which can be a bottleneck in concurrent applications.

## Installation

```bash
go get github.com/thedataflows/go-lib-log
```

## Usage

### Basic Logging

```go
package main

import (
 "errors"
 "os"

 "github.com/thedataflows/go-lib-log/log" // Adjust import path
)

func main() {
 // Create a logger using the builder pattern
 logger := log.NewLoggerBuilder().Build()
 // Ensure to close it on application shutdown to flush buffers
 defer logger.Close()

 logger.Info().Msg("Application started")
 logger.Debug().Msg("This is a debug message.")
 logger.Warn().Str("issue", "potential issue").Msg("Something to be aware of")
 err := errors.New("something went wrong")
 logger.Error().Err(err).Msg("An error occurred")

 // Example of creating logger with custom configuration
 customLogger := log.NewLoggerBuilder().
  WithBufferSize(1000).
  WithRateLimit(500).
  Build()
 defer customLogger.Close()

 // Demonstrate event grouping with repeated messages
 for range 5 {
  logger.Error().Err(errors.New("connection failed")).Msg("Database connection error")
  // These identical messages will be grouped automatically
 }

 // To see Fatal or Panic in action (uncomment to test):
 // logger.Fatal().Err(err).Msg("A fatal error occurred, exiting.")
 // logger.Panic().Err(err).Msg("A panic occurred.")
}

```

### Configuration

The logger can be configured using the following environment variables:

* `LOG_LEVEL`: Sets the minimum log level.
  * Supported values: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic`.
  * Default: `info`.
* `LOG_FORMAT`: Sets the log output format.
  * Supported values: `console`, `json`.
  * Default: `console`.
* `LOG_BUFFER_SIZE`: Sets the size of the internal log message buffer.
  * Default: `100000`.
* `LOG_RATE_LIMIT`: Sets the maximum number of log messages per second.
  * Default: `50000`.
* `LOG_RATE_BURST`: Sets the burst size for the rate limiter.
  * Default: `10000`.
* `LOG_BUFFERING_DISABLED`: Disables buffering and writes log messages directly to output.
  * Supported values: `true`, `1`, `yes` (to disable buffering).
  * Default: `false` (buffering enabled).
  * When disabled, messages are written immediately with rate limiting only.
* `LOG_GROUP_WINDOW`: Sets the time window (in seconds) for grouping similar log events.
  * Default: `1` (1 second, >= 0 enables event grouping with 0 using default value).
  * Set to `-1` to disable event grouping.
* `LOG_DROP_REPORT_INTERVAL`: Sets the interval (in seconds) for reporting dropped log messages.
  * Default: `10`.

## Event Grouping

Event grouping is a powerful feature that reduces log noise by grouping identical messages within a configurable time window. **This feature is enabled by default** with a 1-second grouping window. When multiple identical log messages are received within the window, only the first one is logged immediately, and subsequent identical messages increment a counter. When the window expires, a final grouped message is emitted showing the total count.

### How It Works

1. **Message Hashing**: Each log message is hashed based on its content and level
2. **Time Windows**: Messages are grouped within configurable time windows (default: 1 second)
3. **Atomic Operations**: Uses lock-free atomic operations for high performance
4. **Zero Allocation**: Maintains zero-copy performance where possible

### Grouped Message Format

When messages are grouped, the final log entry includes additional fields:

```json
{
  "time": "2025-06-13T10:30:00Z",
  "level": "error",
  "message": "Database connection failed",
  "group_count": 15,
  "group_window": "1s",
  "group_first": "2025-06-13T10:29:59Z"
}
```

### Using Event Grouping

```go
// Default logger with event grouping enabled (1 second window)
logger := log.NewLoggerBuilder().Build()
defer logger.Close()

// These messages will be grouped if sent within 1 second
logger.Error().Msg("Database connection failed")
logger.Error().Msg("Database connection failed") // Will be grouped
logger.Error().Msg("Database connection failed") // Will be grouped

// Create logger with custom grouping window
logger := log.NewLoggerBuilder().WithGroupWindow(2 * time.Second).Build()
defer logger.Close()

// Create logger with grouping explicitly disabled
logger := log.NewLoggerBuilder().WithoutGrouping().Build()
defer logger.Close()

// Different messages won't be grouped
logger.Error().Msg("Redis connection failed") // Separate message

// Environment variable configuration
os.Setenv(log.ENV_LOG_GROUP_WINDOW, "2") // 2 second window
logger := log.NewLoggerBuilder().Build() // Will use environment configuration

// Disable grouping via environment variable
os.Setenv(log.ENV_LOG_GROUP_WINDOW, "-1") // Disables grouping
logger := log.NewLoggerBuilder().Build()
```

## Buffering Control

The library supports both buffered and unbuffered logging modes to suit different performance and reliability requirements:

### Buffered Mode (Default)

In buffered mode, log messages are queued in an internal buffer and processed asynchronously by a background goroutine. This provides:

* **High Performance**: Minimal impact on application performance
* **High Throughput**: Can handle burst logging scenarios efficiently
* **Backpressure Handling**: Messages are dropped if buffer is full, with periodic drop reports

```go
// Default buffered logger
logger := log.NewLoggerBuilder().Build()
defer logger.Close() // Important: ensures all buffered messages are flushed

logger.Info().Msg("This message goes to the buffer first")
```

### Unbuffered Mode

In unbuffered mode, log messages are written directly to the output with rate limiting only. This provides:

* **Immediate Writing**: Messages appear in logs immediately
* **No Message Loss**: No buffering means no risk of losing messages due to buffer overflow
* **Lower Throughput**: Direct I/O can impact application performance under high load

#### Enable via Environment Variable

```bash
export LOG_BUFFERING_DISABLED=true
# or
export LOG_BUFFERING_DISABLED=1
# or
export LOG_BUFFERING_DISABLED=yes
```

```go
// Will use unbuffered mode due to environment variable
logger := log.NewLoggerBuilder().Build()
logger.Info().Msg("This message is written immediately")
```

#### Enable Programmatically

```go
// Explicitly create unbuffered logger
logger := log.NewLoggerBuilder().WithoutBuffering().Build()
logger.Info().Msg("This message is written immediately")

// Unbuffered with event grouping disabled
logger := log.NewLoggerBuilder().WithoutBuffering().WithoutGrouping().Build()
logger.Info().Msg("Immediate write, no grouping")
```

### When to Use Each Mode

**Use Buffered Mode (Default) When:**

* High logging throughput is required
* Application performance is critical
* Some message loss is acceptable under extreme load
* Logging is primarily for debugging/monitoring

**Use Unbuffered Mode When:**

* Every log message is critical (e.g., audit logs, financial transactions)
* Immediate log visibility is required
* Debugging scenarios where timing is important
* Lower logging volume applications

### Programmatic Configuration

You can also configure the logger programmatically:

```go
package main

import (
 "github.com/thedataflows/go-lib-log/log" // Adjust import path
 "github.com/rs/zerolog"
 "os"
 "time"
)

func main() {
 // Create logger with programmatic configuration
 logger := log.NewLoggerBuilder().
  WithLogLevelString("debug").
  WithBufferSize(10000).
  WithRateLimit(1000).
  Build()
 defer logger.Close()

 logger.Info().Msg("Logger configured programmatically.")

 // For more advanced custom logger setup with custom output:
 customOutput := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
 customLogger := log.NewLoggerBuilder().
  WithOutput(customOutput).
  WithLogLevel(zerolog.InfoLevel).
  Build()
 defer customLogger.Close()

 customLogger.Info().Msg("This is from a custom logger instance")
}
```

### Logger Configuration with Builder Pattern

The library uses a builder pattern for creating loggers with flexible configuration:

```go
// Default logger with event grouping enabled (1 second window)
logger := log.NewLoggerBuilder().Build()

// Logger with custom event grouping window
logger := log.NewLoggerBuilder().WithGroupWindow(5 * time.Second).Build()

// Logger with event grouping explicitly disabled
logger := log.NewLoggerBuilder().WithGroupWindow(-1).Build()

// JSON-only logger (always outputs JSON format)
logger := log.NewLoggerBuilder().WithLogFormat(log.LOG_FORMAT_JSON).Build()

// High-performance buffered logger with custom configuration
logger := log.NewLoggerBuilder().
    WithBufferSize(50000).
    WithRateLimit(10000).
    WithGroupWindow(2 * time.Second).
    Build()

// Unbuffered logger for critical logging
logger := log.NewLoggerBuilder().
    WithoutBuffering().
    WithoutGrouping().
    Build()
```

## Important: Close() Method

**Always call `Close()` on your logger instances** to ensure proper cleanup and message flushing:

```go
logger := log.NewLoggerBuilder().Build()
defer logger.Close() // Critical for proper cleanup
```

The `Close()` method performs several important operations:

1. **Flushes Pending Grouped Messages**: Any messages waiting in the event grouper are immediately flushed
2. **Processes Buffered Messages**: All messages in the internal buffer are processed through rate limiting
3. **Stops Background Goroutines**: Cleanly shuts down the processor and drop reporter goroutines
4. **Prevents Message Loss**: Ensures no messages are lost during application shutdown

### Close() Behavior

* **Thread-Safe**: Can be called multiple times safely (subsequent calls are no-ops)
* **Blocking**: Will wait for all pending messages to be processed
* **Rate-Limited**: Pending grouped messages go through normal rate limiting during flush
* **Atomic**: Uses atomic operations to prevent race conditions during shutdown

### Example with Proper Cleanup

```go
func main() {
    logger := log.NewLoggerBuilder().Build()
    defer logger.Close() // Ensures all messages are flushed

    // Send many identical messages - these will be grouped
    for range 100 {
        logger.Error().Msg("Database connection failed")
    }

    // Close() will flush the grouped message before exiting
    // Without Close(), the grouped message might be lost
}
```

**Note**: The global logger used by package-level functions (`log.Info()`, `log.Error()`, etc.) should also be closed with `log.Close()` for proper cleanup.

### Using the Global Logger vs Instance Logger

You can use the library in two ways:

```go
// Option 1: Global logger (backwards compatible)
import log "github.com/thedataflows/go-lib-log"

func main() {
    defer log.Close() // Close global logger
    log.Info("pkg", "Using global logger")
}

// Option 2: Instance logger (recommended for new code)
import log "github.com/thedataflows/go-lib-log"

func main() {
    logger := log.NewLoggerBuilder().Build()
    defer logger.Close()
    logger.Info().Msg("Using instance logger")
}
```

### Stack Traces

The library now includes a stack trace marshaller that outputs the current stack trace to stderr whenever an error is logged. This is useful for debugging and understanding the context of errors.
`Stack()` must be called before `Err()`.

```go
logger.Error().Stack().Err(errors.New("something went wrong")).Msg("An error occurred")
```

## License

[MIT License](LICENSE)
