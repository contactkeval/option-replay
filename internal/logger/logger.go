// Package logger provides a lightweight, centralized logging facility
// with configurable verbosity levels.
//
// Design goals:
//   - Simple API (Errorf, Infof, Debugf, Tracef)
//   - Centralized verbosity control
//   - Zero formatting logic at call sites
//   - Leverages Go's standard log package
//
// Verbosity levels (in increasing order):
//
//	Error < Info < Debug < Trace
//
// Example usage:
//
//	logger.SetVerbosity(2) // Debug
//	logger.Infof("starting engine")
//	logger.Debugf("spot=%f vol=%f", spot, vol)
package logger

import (
	"log"
	"os"
)

// Level represents a logging verbosity level.
// Higher values mean more verbose logging.
type Level int

const (
	Error Level = iota // Error logs only critical failures.
	Info               // Info logs high-level application progress.
	Debug              // Debug logs detailed diagnostic information.
	Trace              // Trace logs very fine-grained execution details.
)

// current holds the active verbosity level.
// Only messages with level <= current are logged.
var current Level = Info

// init configures the global logger used by this package.
//
// init() is executed automatically when the package is imported,
// before any other code runs. This makes it ideal for one-time,
// package-wide setup such as logging configuration.
func init() {
	// Write all log output to standard error (stderr).
	// This ensures logs are separated from normal program output,
	// which is especially important for CLI tools and pipelines.
	log.SetOutput(os.Stderr)

	// Configure log formatting:
	//   - log.LstdFlags  → date and time (YYYY/MM/DD HH:MM:SS)
	//   - log.Lshortfile → source file name and line number
	//
	// Example output:
	//   2026/01/25 15:42:10 engine.go:87 [INFO] pricing started
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

// SetVerbosity sets the global logging verbosity.
// Typically called once during application startup
// (e.g. after parsing CLI flags).
func SetVerbosity(v int) {
	current = Level(v)
}

// logf is the internal logging helper.
// It checks verbosity and delegates formatting/output
// to the standard library logger.
func logf(l Level, prefix, format string, args ...any) {
	if current >= l {
		log.Printf(prefix+format, args...)
	}
}

// Errorf logs an error-level message.
// Use this for failures that require attention.
func Errorf(format string, args ...any) {
	logf(Error, "[ERROR] ", format, args...)
}

// Infof logs an informational message.
// Use this for major lifecycle events.
func Infof(format string, args ...any) {
	logf(Info, "[INFO]  ", format, args...)
}

// Debugf logs debugging information.
// Use this for diagnostic output useful during development.
func Debugf(format string, args ...any) {
	logf(Debug, "[DEBUG] ", format, args...)
}

// Tracef logs very detailed execution traces.
// Use this sparingly due to high volume.
func Tracef(format string, args ...any) {
	logf(Trace, "[TRACE] ", format, args...)
}
