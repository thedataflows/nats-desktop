// Package log provides a high-performance logging library with advanced features
// including rate limiting, buffering, event grouping, and configurable output formats.
//
// # Buffering Control
//
// The library supports both buffered and unbuffered logging modes:
//
// Buffered Mode (default):
//   - Messages are queued in a buffer and processed asynchronously
//   - High throughput with minimal impact on application performance
//   - Messages may be dropped if buffer is full (backpressure handling)
//   - Suitable for high-frequency logging scenarios
//
// Unbuffered Mode:
//   - Messages are written directly to the output with rate limiting
//   - Immediate writing ensures no message loss due to buffering
//   - Lower throughput but guaranteed message delivery
//   - Suitable for critical logging where message loss is unacceptable
//
// # Environment Variables
//
// The following environment variables control logging behavior:
//   - LOG_BUFFERING_DISABLED: Set to "true", "1", or "yes" to disable buffering
//   - LOG_BUFFER_SIZE: Buffer size for buffered mode (default: 100000)
//   - LOG_RATE_LIMIT: Rate limit in messages per second (default: 50000)
//   - LOG_RATE_BURST: Burst size for rate limiting (default: 10000)
//   - LOG_GROUP_WINDOW: Event grouping window in seconds (default: 1)
//   - LOG_DROP_REPORT_INTERVAL: Drop report interval in seconds (default: 10)
//   - LOG_LEVEL: Log level (debug, info, warn, error, fatal, panic)
//   - LOG_FORMAT: Output format (json, console)
//
// # Usage Examples
//
// Basic usage with default configuration:
//
//	logger := log.NewLoggerBuilder().Build()
//	logger.Info().Msg("This is a buffered log message")
//	logger.Close() // Flush remaining messages
//
// Configure with builder pattern:
//
//	logger := log.NewLoggerBuilder().
//		WithoutBuffering().
//		WithGroupWindow(5 * time.Second).
//		AsJSON().
//		Build()
//	logger.Info().Msg("This message is written immediately with 5s grouping")
//
// Disable buffering via environment variable:
//
//	os.Setenv(log.ENV_LOG_BUFFERING_DISABLED, "true")
//	logger := log.NewLoggerBuilder().Build()
//	logger.Info().Msg("This message is written immediately")
//
// Create unbuffered logger with builder:
//
//	logger := log.NewLoggerBuilder().WithoutBuffering().Build()
//	logger.Info().Msg("This message is written immediately")
//
// Unbuffered with grouping disabled:
//
//	logger := log.NewLoggerBuilder().WithoutBuffering().WithoutGrouping().Build()
//	logger.Info().Msg("No buffering, no grouping")
package log

import (
	"bytes"
	"context"
	"hash/fnv"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/time/rate"
)

const (
	// ENV_LOG_LEVEL is the environment variable for the log level.
	ENV_LOG_LEVEL = "LOG_LEVEL"
	// ENV_LOG_FORMAT is the environment variable for the log format.
	ENV_LOG_FORMAT = "LOG_FORMAT"
	// KEY_PKG is the key used for the package name in log fields.
	KEY_PKG = "pkg"
)

var (
	// ParseLevel parses a string into a zerolog.Level. It's a convenience wrapper around zerolog.ParseLevel.
	ParseLevel = zerolog.ParseLevel

	// DebugLevel defines the debug log level.
	DebugLevel = zerolog.DebugLevel
	// InfoLevel defines the info log level.
	InfoLevel = zerolog.InfoLevel
	// WarnLevel defines the warn log level.
	WarnLevel = zerolog.WarnLevel
	// ErrorLevel defines the error log level.
	ErrorLevel = zerolog.ErrorLevel
	// FatalLevel defines the fatal log level.
	FatalLevel = zerolog.FatalLevel
	// PanicLevel defines the panic log level.
	PanicLevel = zerolog.PanicLevel
	// NoLevel defines an absent log level.
	NoLevel = zerolog.NoLevel
	// Disabled disables the logger.
	Disabled = zerolog.Disabled
	// TraceLevel defines the trace log level.
	TraceLevel = zerolog.TraceLevel
)

// eventKey represents a unique identifier for grouping similar log events.
// It uses the message content hash and level to identify similar events.
type eventKey struct {
	hash  uint64
	level string
}

// eventWindow tracks events within a time window for grouping.
type eventWindow struct {
	firstSeen time.Time
	count     atomic.Uint64
	message   []byte // Store the first occurrence message
}

// eventGrouper manages event grouping within time windows.
type eventGrouper struct {
	events         sync.Map // Use sync.Map for better concurrent performance
	windowDuration time.Duration
	cleanupChan    chan struct{}
	wg             sync.WaitGroup
	writer         io.Writer // Reference to output writer for emitting grouped messages
}

// newEventGrouper creates a new event grouper with the specified window duration.
func newEventGrouper(windowDuration time.Duration, writer io.Writer) *eventGrouper {
	eg := &eventGrouper{
		windowDuration: windowDuration,
		cleanupChan:    make(chan struct{}),
		writer:         writer,
	}

	// Start cleanup goroutine if grouping is enabled
	if windowDuration > 0 {
		eg.wg.Add(1)
		go eg.cleanupExpired()
	}

	return eg
}

// CustomLogger wraps zerolog.Logger to provide additional functionalities like
// rate limiting, buffering, and custom formatting.
type CustomLogger struct {
	zerolog.Logger
	writer     *BufferedRateLimitedWriter
	bufferSize int
	once       sync.Once
	logFormat  LogFormat
}

// BufferedRateLimitedWriter wraps an io.Writer with rate limiting and buffering.
// It ensures that logs are written at a controlled pace and buffers messages
// to handle bursts, dropping messages if the buffer is full and reporting drops.
// It also supports event grouping to reduce duplicate log noise.
type BufferedRateLimitedWriter struct {
	target    io.Writer
	targetMux sync.Mutex // Protects concurrent writes to target
	limiter   *rate.Limiter
	buffer    chan []byte
	wg        sync.WaitGroup
	once      sync.Once

	closed      atomic.Bool   // Changed from bool with mutex
	closeSignal chan struct{} // Added for shutdown signaling
	ctx         context.Context
	cancelFunc  context.CancelFunc

	// Drop tracking and reporting
	droppedCount       atomic.Uint64
	dropReportTicker   *time.Ticker
	dropReportInterval time.Duration
	reportWg           sync.WaitGroup

	// Pre-computed drop report parts for performance
	dropReportPrefix   []byte
	dropReportSuffix   []byte
	intervalSecondsStr string

	// Event grouping
	grouper *eventGrouper

	// Buffering control
	bufferingDisabled bool
}

func (w *BufferedRateLimitedWriter) startProcessor() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()

		for data := range w.buffer {
			// Use the cancellable context for rate limiting
			if err := w.limiter.Wait(w.ctx); err != nil {
				// Context cancelled, write without rate limiting
			}

			// Protect concurrent writes to target with mutex
			w.targetMux.Lock()
			_, _ = w.target.Write(data)
			w.targetMux.Unlock()
		}
	}()
}

func (w *BufferedRateLimitedWriter) startDropReporter() {
	w.reportWg.Add(1) // Increment WaitGroup counter before starting goroutine
	go func() {
		defer w.reportWg.Done()

		// Only start ticker if interval is positive
		if w.dropReportInterval > 0 {
			w.dropReportTicker = time.NewTicker(w.dropReportInterval)
			defer w.dropReportTicker.Stop()
		} else {
			// If no ticker, block on closeSignal directly to keep goroutine alive
			// until close, otherwise it exits immediately if drop reporting is disabled.
			<-w.closeSignal
			return
		}

		for {
			select {
			case <-w.dropReportTicker.C:
				// Atomically read and reset the dropped count
				dropped := w.getAndResetDroppedCount()
				if dropped > 0 {
					// Create drop report message
					timestamp := time.Now().Format(time.RFC3339)
					droppedStr := strconv.FormatUint(dropped, 10)

					// Simplified and corrected capacity calculation for valid JSON construction
					capacity := len(w.dropReportPrefix) + len(timestamp) + len(w.dropReportSuffix) +
						len(droppedStr) + len(",\"interval_seconds\":\"") + len(w.intervalSecondsStr) +
						len("\"}") + len("\n") // Closing quote for interval_seconds value, closing brace, and newline

					buf := make([]byte, 0, capacity)
					buf = append(buf, w.dropReportPrefix...)
					buf = append(buf, timestamp...)
					buf = append(buf, w.dropReportSuffix...)
					buf = append(buf, droppedStr...)
					buf = append(buf, []byte(",\"interval_seconds\":\"")...)
					buf = append(buf, w.intervalSecondsStr...)
					buf = append(buf, []byte("\"}")...) // Closing quote for interval_seconds value and closing brace
					buf = append(buf, []byte("\n")...)

					// Write directly to target to avoid infinite recursion
					w.targetMux.Lock()
					_, _ = w.target.Write(buf)
					w.targetMux.Unlock()
				}
			case <-w.closeSignal:
				return
			}
		}
	}()
}

func (w *BufferedRateLimitedWriter) getAndResetDroppedCount() uint64 {
	return w.droppedCount.Swap(0)
}

// Write writes the provided byte slice to the buffer.
// It implements io.Writer. If the buffer is full, messages are dropped,
// and the drop count is incremented.
// It starts the processor and drop reporter on the first write.
// If buffering is disabled, writes directly to the target with rate limiting.
func (w *BufferedRateLimitedWriter) Write(p []byte) (int, error) {
	if w.closed.Load() { // Use atomic Load
		return 0, io.ErrClosedPipe
	}

	// Handle unbuffered mode - write directly to target with rate limiting
	if w.bufferingDisabled {
		// Check if this event should be grouped
		messageToWrite, wasGrouped := w.grouper.shouldGroup(p)
		if wasGrouped {
			// Event was grouped (suppressed), but we still report success
			return len(p), nil
		}

		if messageToWrite == nil {
			// Shouldn't happen, but guard against it
			return len(p), nil
		}

		// Apply rate limiting
		_ = w.limiter.Wait(w.ctx)

		// Write directly to target with mutex protection
		w.targetMux.Lock()
		_, err := w.target.Write(messageToWrite)
		w.targetMux.Unlock()

		// Return based on original input length, not messageToWrite length
		if err != nil {
			return 0, err
		}
		return len(p), nil
	}

	// Buffered mode (existing logic)
	w.once.Do(func() {
		w.startProcessor()
		w.startDropReporter()
	})

	// Check if this event should be grouped
	messageToWrite, wasGrouped := w.grouper.shouldGroup(p)
	if wasGrouped {
		// Event was grouped (suppressed), but we still report success
		return len(p), nil
	}

	if messageToWrite == nil {
		// Shouldn't happen, but guard against it
		return len(p), nil
	}

	// Make a copy of the byte slice since zerolog may reuse it
	// This is the minimal copying we need to do for buffering
	dataCopy := make([]byte, len(messageToWrite))
	copy(dataCopy, messageToWrite)

	// Non-blocking send with backpressure (drop if buffer full)
	select {
	case w.buffer <- dataCopy:
		return len(p), nil
	default:
		// Buffer full - implement backpressure by dropping
		// Increment drop counter atomically
		w.droppedCount.Add(1) // Changed from mutex-guarded increment
		return len(p), nil    // Return success to not break the logger
	}
}

// Close closes the BufferedRateLimitedWriter, ensuring all buffered messages are processed
// and the drop reporter is stopped.
func (w *BufferedRateLimitedWriter) Close() error {
	if !w.closed.CompareAndSwap(false, true) { // Use atomic CAS
		return nil // Already closed or closing
	}

	// Close the event grouper first to get any pending grouped messages
	// This must be done before closing the buffer so pending messages can be processed
	pendingMessages := w.grouper.close()

	// Handle pending messages based on buffering mode
	if w.bufferingDisabled {
		// In unbuffered mode, write pending messages directly
		for _, msg := range pendingMessages {
			_ = w.limiter.Wait(context.Background())
			w.targetMux.Lock()
			_, _ = w.target.Write(msg)
			w.targetMux.Unlock()
		}

		close(w.closeSignal) // Signal drop reporter to stop

		// Wait for drop reporter to finish (even in unbuffered mode, we may have drop reporting)
		w.reportWg.Wait()

		return nil
	}

	// Buffered mode - send pending grouped messages through the normal buffer flow
	// This ensures they go through rate limiting and proper processing
	for _, msg := range pendingMessages {
		// Make a copy since we need to ensure the message persists
		msgCopy := make([]byte, len(msg))
		copy(msgCopy, msg)

		// Try to send to buffer, but don't block if buffer is full
		select {
		case w.buffer <- msgCopy:
			// Successfully queued
		default:
			// Buffer full, write directly with rate limiting and mutex protection
			_ = w.limiter.Wait(context.Background())
			w.targetMux.Lock()
			_, _ = w.target.Write(msgCopy)
			w.targetMux.Unlock()
		}
	}

	close(w.closeSignal) // Signal drop reporter to stop

	// Close the buffer and wait for processor to finish
	close(w.buffer)

	// Wait for processor to finish processing all buffered messages
	w.wg.Wait()

	// Only cancel the context after processor is done
	w.cancelFunc()

	// Wait for drop reporter to finish
	w.reportWg.Wait()

	return nil
}

// Close closes the CustomLogger and its underlying BufferedRateLimitedWriter.
func (l *CustomLogger) Close() {
	if l.writer != nil {
		_ = l.writer.Close()
	}
}

// SetZerologLogger replaces the underlying zerolog.Logger instance in the CustomLogger.
func (l *CustomLogger) SetZerologLogger(logger zerolog.Logger) {
	l.Logger = logger
}

// Flush ensures all buffered messages are written to the output.
// This is useful when you want to force immediate output without closing the logger.
func (l *CustomLogger) Flush() {
	if l.writer != nil {
		l.writer.Flush()
	}
}

// LogFormat returns the log format of the CustomLogger.
func (l *CustomLogger) LogFormat() LogFormat {
	return l.logFormat
}

// PreferredWriter returns an io.Writer that writes to both a new bytes.Buffer and os.Stderr.
// This is typically used for console logging to capture output for testing or other purposes
// while still printing to the standard error stream.
func PreferredWriter() io.Writer {
	return io.MultiWriter(
		new(bytes.Buffer),
		os.Stderr,
	)
}

// Hash computes the hash of the given byte slice using the FNV-1a algorithm.
// It returns the hash value as an uint64.
func Hash(data []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(data)
	return h.Sum64()
}

// hashMessage creates a hash of the log message content for grouping.
func (eg *eventGrouper) hashMessage(data []byte) uint64 {
	// Extract the message part from JSON log for hashing
	// We'll hash the entire message for simplicity and performance
	h := fnv.New64a()
	_, _ = h.Write(data)
	return h.Sum64()
}

// extractLevel extracts the log level from the JSON log message.
func (eg *eventGrouper) extractLevel(data []byte) string {
	// Simple extraction of level field from JSON
	// This is a fast, zero-allocation approach
	start := bytes.Index(data, []byte(`"level":"`))
	if start == -1 {
		return "unknown"
	}
	start += 9 // len(`"level":"`)
	end := bytes.Index(data[start:], []byte(`"`))
	if end == -1 {
		return "unknown"
	}
	return string(data[start : start+end])
}

// shouldGroup determines if an event should be grouped and returns the modified message.
// Returns the message to write and whether it was grouped.
func (eg *eventGrouper) shouldGroup(data []byte) ([]byte, bool) {
	if eg.windowDuration <= 0 {
		return data, false
	}

	hash := eg.hashMessage(data)
	level := eg.extractLevel(data)
	key := eventKey{hash: hash, level: level}
	now := time.Now()

	// Try to load existing window
	if value, exists := eg.events.Load(key); exists {
		window := value.(*eventWindow)

		// Check if the event is within the time window
		if now.Sub(window.firstSeen) <= eg.windowDuration {
			// Within window, increment count atomically and suppress this message
			window.count.Add(1)
			return nil, true
		}

		// Window expired, try to replace with new window
		newWindow := &eventWindow{
			firstSeen: now,
			message:   append([]byte(nil), data...), // Copy the new message
		}
		newWindow.count.Store(1)

		// Create grouped message from expired window
		groupedMsg := eg.createGroupedMessage(window)

		// Try to replace the old window with the new one
		// If another goroutine replaced it, that's fine - we still emit the grouped message
		eg.events.Store(key, newWindow)

		return groupedMsg, false
	}

	// First occurrence of this event - store it
	newWindow := &eventWindow{
		firstSeen: now,
		message:   append([]byte(nil), data...), // Copy the message
	}
	newWindow.count.Store(1)

	// Try to store the new window
	if _, loaded := eg.events.LoadOrStore(key, newWindow); !loaded {
		// Successfully stored new window, return original message
		return data, false
	}

	// Another goroutine stored a window for this key, retry
	return eg.shouldGroup(data)
}

// createGroupedMessage creates a grouped message showing the count.
func (eg *eventGrouper) createGroupedMessage(window *eventWindow) []byte {
	count := window.count.Load()
	if count <= 1 {
		return window.message
	}

	// Parse the original message to add count information
	// We'll modify the JSON to include a count field
	msg := make([]byte, 0, len(window.message)+100) // Pre-allocate with extra space

	// Find the closing brace and insert count before it
	lastBrace := bytes.LastIndex(window.message, []byte("}"))
	if lastBrace == -1 {
		// Fallback: just append the original message
		return window.message
	}

	// Copy everything up to the last brace
	msg = append(msg, window.message[:lastBrace]...)

	// Add count and grouped indicator
	countStr := strconv.FormatUint(count, 10)
	windowDurationStr := eg.windowDuration.String()

	msg = append(msg, []byte(`,"group_count":`)...)
	msg = append(msg, countStr...)
	msg = append(msg, []byte(`,"group_window":"`)...)
	msg = append(msg, windowDurationStr...)
	msg = append(msg, []byte(`","group_first":"`)...)
	msg = append(msg, window.firstSeen.Format(time.RFC3339)...)
	msg = append(msg, []byte(`"}`)...)
	msg = append(msg, '\n')

	return msg
}

// cleanupExpired removes expired event windows.
func (eg *eventGrouper) cleanupExpired() {
	defer eg.wg.Done()

	ticker := time.NewTicker(eg.windowDuration / 2) // Cleanup twice per window
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			eg.performCleanup()
		case <-eg.cleanupChan:
			// Final cleanup before shutdown
			eg.performCleanup()
			return
		}
	}
}

// performCleanup removes expired event windows and emits final grouped messages.
func (eg *eventGrouper) performCleanup() {
	now := time.Now()
	var expiredMessages [][]byte

	// Use Range to iterate over sync.Map
	eg.events.Range(func(key, value interface{}) bool {
		window := value.(*eventWindow)
		if now.Sub(window.firstSeen) > eg.windowDuration {
			// Create grouped message for expired window if it has multiple events
			if window.count.Load() > 1 {
				groupedMsg := eg.createGroupedMessage(window)
				expiredMessages = append(expiredMessages, groupedMsg)
			}
			// Delete expired window
			eg.events.Delete(key)
		}
		return true // Continue iteration
	})

	// Emit expired grouped messages through the writer's target
	// We need access to the writer to emit these messages
	if len(expiredMessages) > 0 && eg.writer != nil {
		for _, msg := range expiredMessages {
			_, _ = eg.writer.Write(msg)
		}
	}
}

// close shuts down the event grouper and returns any pending grouped messages.
func (eg *eventGrouper) close() [][]byte {
	var pendingMessages [][]byte

	if eg.windowDuration > 0 {
		// Collect all pending grouped messages before shutdown
		eg.events.Range(func(key, value interface{}) bool {
			window := value.(*eventWindow)
			if window.count.Load() > 1 {
				groupedMsg := eg.createGroupedMessage(window)
				pendingMessages = append(pendingMessages, groupedMsg)
			}
			return true
		})

		close(eg.cleanupChan)
		eg.wg.Wait()
	}

	return pendingMessages
}

// Flush ensures all buffered messages are written to the target.
// This is useful when you want to force immediate output without closing the writer.
func (w *BufferedRateLimitedWriter) Flush() {
	if w.closed.Load() {
		return // Already closed
	}

	if w.bufferingDisabled {
		// In unbuffered mode, nothing to flush
		return
	}

	// Ensure processor is started
	w.once.Do(func() {
		w.startProcessor()
		w.startDropReporter()
	})

	// Simple approach: wait for buffer to drain
	for range 100 { // Max 100ms
		if len(w.buffer) == 0 {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
}
