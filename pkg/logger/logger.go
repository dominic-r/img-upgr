package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
)

// TimeFormat defines the standard time format used in log messages
const TimeFormat = "2006-01-02 15:04:05"

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	// DEBUG level for detailed troubleshooting information
	DEBUG LogLevel = iota
	// INFO level for general operational information
	INFO
	// WARN level for potentially harmful situations
	WARN
	// ERROR level for error events that might still allow the application to continue
	ERROR
	// FATAL level for very severe error events that will lead to application termination
	FATAL
)

var (
	// defaultLogger is the global logger instance
	defaultLogger *Logger

	// Color functions for different log levels
	debugColor = color.New(color.FgCyan).SprintFunc()
	infoColor  = color.New(color.FgGreen).SprintFunc()
	warnColor  = color.New(color.FgYellow).SprintFunc()
	errorColor = color.New(color.FgRed).SprintFunc()
	fatalColor = color.New(color.FgHiRed, color.Bold).SprintFunc()
)

// Logger represents a logger with configurable level and output
type Logger struct {
	level       LogLevel
	output      io.Writer
	quiet       bool
	useColors   bool
	errorOutput io.Writer
}

// LoggerOption defines a function that modifies a Logger
type LoggerOption func(*Logger)

// WithErrorOutput sets a separate writer for error and fatal logs
func WithErrorOutput(w io.Writer) LoggerOption {
	return func(l *Logger) {
		l.errorOutput = w
	}
}

// WithoutColors disables colored output
func WithoutColors() LoggerOption {
	return func(l *Logger) {
		l.useColors = false
	}
}

// WithQuiet enables quiet mode (only ERROR and FATAL messages)
func WithQuiet() LoggerOption {
	return func(l *Logger) {
		l.quiet = true
	}
}

// init initializes the default logger
func init() {
	defaultLogger = NewLogger(INFO, os.Stdout)
}

// NewLogger creates a new logger with the specified level and output
func NewLogger(level LogLevel, output io.Writer, options ...LoggerOption) *Logger {
	logger := &Logger{
		level:       level,
		output:      output,
		quiet:       false,
		useColors:   true,
		errorOutput: output, // Default error output is the same as normal output
	}

	// Apply options
	for _, option := range options {
		option(logger)
	}

	return logger
}

// SetLevel sets the log level for the default logger
func SetLevel(level LogLevel) {
	defaultLogger.level = level
}

// SetQuiet sets the quiet mode for the default logger
func SetQuiet(quiet bool) {
	defaultLogger.quiet = quiet
}

// SetOutput sets the output writer for the default logger
func SetOutput(w io.Writer) {
	defaultLogger.output = w
}

// DisableColors disables colored output for the default logger
func DisableColors() {
	defaultLogger.useColors = false
}

// GetLevel returns the current log level as a string
func GetLevel() string {
	return defaultLogger.level.String()
}

// String returns the string representation of a log level
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses a log level string into a LogLevel
func ParseLevel(level string) LogLevel {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	case "FATAL":
		return FATAL
	default:
		return INFO
	}
}

// log logs a message at the specified level
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if l.quiet && level < ERROR {
		return
	}

	if level < l.level {
		return
	}

	timestamp := time.Now().Format(TimeFormat)
	levelStr := level.String()

	var coloredLevel string
	if l.useColors {
		switch level {
		case DEBUG:
			coloredLevel = debugColor(levelStr)
		case INFO:
			coloredLevel = infoColor(levelStr)
		case WARN:
			coloredLevel = warnColor(levelStr)
		case ERROR:
			coloredLevel = errorColor(levelStr)
		case FATAL:
			coloredLevel = fatalColor(levelStr)
		default:
			coloredLevel = levelStr
		}
	} else {
		coloredLevel = levelStr
	}

	message := fmt.Sprintf(format, args...)
	logLine := fmt.Sprintf("%s [%s] %s\n", timestamp, coloredLevel, message)

	// Use errorOutput for ERROR and FATAL levels if set
	if (level == ERROR || level == FATAL) && l.errorOutput != nil {
		if _, err := fmt.Fprint(l.errorOutput, logLine); err != nil {
			// Can't do much if logging itself fails, but at least try to write to stderr
			_, _ = fmt.Fprintf(os.Stderr, "Error writing to log: %v\n", err)
		}
	} else {
		if _, err := fmt.Fprint(l.output, logLine); err != nil {
			// Can't do much if logging itself fails, but at least try to write to stderr
			_, _ = fmt.Fprintf(os.Stderr, "Error writing to log: %v\n", err)
		}
	}

	if level == FATAL {
		os.Exit(1)
	}
}

// Debug logs a formatted debug message
func Debug(format string, args ...interface{}) {
	defaultLogger.log(DEBUG, format, args...)
}

// Info logs a formatted info message
func Info(format string, args ...interface{}) {
	defaultLogger.log(INFO, format, args...)
}

// Warn logs a formatted warning message
func Warn(format string, args ...interface{}) {
	defaultLogger.log(WARN, format, args...)
}

// Error logs a formatted error message
func Error(format string, args ...interface{}) {
	defaultLogger.log(ERROR, format, args...)
}

// Fatal logs a formatted fatal message and exits the application
func Fatal(format string, args ...interface{}) {
	defaultLogger.log(FATAL, format, args...)
}

// Debugf logs a formatted debug message (alias for Debug for consistency)
func Debugf(format string, args ...interface{}) {
	defaultLogger.log(DEBUG, format, args...)
}

// Infof logs a formatted info message (alias for Info for consistency)
func Infof(format string, args ...interface{}) {
	defaultLogger.log(INFO, format, args...)
}

// Warnf logs a formatted warning message (alias for Warn for consistency)
func Warnf(format string, args ...interface{}) {
	defaultLogger.log(WARN, format, args...)
}

// Errorf logs a formatted error message (alias for Error for consistency)
func Errorf(format string, args ...interface{}) {
	defaultLogger.log(ERROR, format, args...)
}

// Fatalf logs a formatted fatal message and exits (alias for Fatal for consistency)
func Fatalf(format string, args ...interface{}) {
	defaultLogger.log(FATAL, format, args...)
}

// Debugln logs a debug message without formatting
func Debugln(args ...interface{}) {
	defaultLogger.log(DEBUG, "%s", fmt.Sprint(args...))
}

// Infoln logs an info message without formatting
func Infoln(args ...interface{}) {
	defaultLogger.log(INFO, "%s", fmt.Sprint(args...))
}

// Warnln logs a warning message without formatting
func Warnln(args ...interface{}) {
	defaultLogger.log(WARN, "%s", fmt.Sprint(args...))
}

// Errorln logs an error message without formatting
func Errorln(args ...interface{}) {
	defaultLogger.log(ERROR, "%s", fmt.Sprint(args...))
}

// Fatalln logs a fatal message without formatting and exits
func Fatalln(args ...interface{}) {
	defaultLogger.log(FATAL, "%s", fmt.Sprint(args...))
}
