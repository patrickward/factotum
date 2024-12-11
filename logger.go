package faktotum

import (
	"fmt"
	"log/slog"
	"strings"
)

// FaktoryLogger is a logger that implements the Logger interface from the faktory_worker package.
// It wraps the slog.Logger type from the slog package.
type FaktoryLogger struct {
	logger *slog.Logger
	prefix string
}

// NewFaktoryLogger creates a new FaktoryLogger.
func NewFaktoryLogger(logger *slog.Logger) *FaktoryLogger {
	return &FaktoryLogger{
		logger: logger,
		prefix: "[Faktory] ",
	}
}

// convertArgsToMessage converts variadic arguments to a string message
func (l *FaktoryLogger) convertArgsToMessage(v ...interface{}) string {
	// Convert all arguments to strings and join them
	parts := make([]string, len(v))
	for i, arg := range v {
		parts[i] = fmt.Sprint(arg)
	}
	return l.prefix + strings.Join(parts, " ")
}

// Debug logs a message at the debug level.
func (l *FaktoryLogger) Debug(v ...interface{}) {
	msg := l.convertArgsToMessage(v...)
	l.logger.Debug(msg)
}

// Debugf logs a message at the debug level with a format string.
func (l *FaktoryLogger) Debugf(format string, args ...interface{}) {
	l.logger.Debug(l.prefix + fmt.Sprintf(format, args...))
}

// Info logs a message at the info level.
func (l *FaktoryLogger) Info(v ...interface{}) {
	msg := l.convertArgsToMessage(v...)
	l.logger.Info(msg)
}

// Infof logs a message at the info level with a format string.
func (l *FaktoryLogger) Infof(format string, args ...interface{}) {
	l.logger.Info(l.prefix + fmt.Sprintf(format, args...))
}

// Warn logs a message at the warn level.
func (l *FaktoryLogger) Warn(v ...interface{}) {
	msg := l.convertArgsToMessage(v...)
	l.logger.Warn(msg)
}

// Warnf logs a message at the warn level with a format string.
func (l *FaktoryLogger) Warnf(format string, args ...interface{}) {
	l.logger.Warn(l.prefix + fmt.Sprintf(format, args...))
}

// Error logs a message at the error level.
func (l *FaktoryLogger) Error(v ...interface{}) {
	msg := l.convertArgsToMessage(v...)
	l.logger.Error(msg)
}

// Errorf logs a message at the error level with a format string.
func (l *FaktoryLogger) Errorf(format string, args ...interface{}) {
	l.logger.Error(l.prefix + fmt.Sprintf(format, args...))
}

// Fatal logs a message at the fatal level and exits the program.
func (l *FaktoryLogger) Fatal(v ...interface{}) {
	msg := l.convertArgsToMessage(v...)
	l.logger.Error(msg, slog.String("level", "fatal"))
}

// Fatalf logs a message at the fatal level with a format string and exits the program.
func (l *FaktoryLogger) Fatalf(format string, args ...interface{}) {
	l.logger.Error(l.prefix+fmt.Sprintf(format, args...), slog.String("level", "fatal"))
}
