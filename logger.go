package faktotum

import (
	"fmt"
	"log/slog"
)

// FaktoryLogger is a logger that implements the Logger interface from the faktory_worker package.
// Its wraps the slog.Logger type from the slog package.
type FaktoryLogger struct {
	logger *slog.Logger
}

// NewFaktoryLogger creates a new FaktoryLogger.
func NewFaktoryLogger(logger *slog.Logger) *FaktoryLogger {
	return &FaktoryLogger{logger: logger}
}

// Debug logs a message at the debug level.
func (l *FaktoryLogger) Debug(v ...interface{}) {
	l.logger.Debug("Faktory Debug", v...)
}

// Debugf logs a message at the debug level with a format string.
func (l *FaktoryLogger) Debugf(format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	l.logger.Debug(message)
}

// Info logs a message at the info level.
func (l *FaktoryLogger) Info(v ...interface{}) {
	l.logger.Info("Faktory Info", v...)
}

// Infof logs a message at the info level with a format string.
func (l *FaktoryLogger) Infof(format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	l.logger.Info(message)
}

// Warn logs a message at the warn level.
func (l *FaktoryLogger) Warn(v ...interface{}) {
	l.logger.Warn("Faktory Warn", v...)
}

// Warnf logs a message at the warn level with a format string.
func (l *FaktoryLogger) Warnf(format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	l.logger.Warn(message)
}

// Error logs a message at the error level.
func (l *FaktoryLogger) Error(v ...interface{}) {
	l.logger.Error("Faktory Error", v...)
}

// Errorf logs a message at the error level with a format string.
func (l *FaktoryLogger) Errorf(format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	l.logger.Error(message)
}

// Fatal logs a message at the fatal level.
func (l *FaktoryLogger) Fatal(v ...interface{}) {
	l.logger.Error("Faktory Fatal", v...)
}

// Fatalf logs a message at the fatal level with a format string.
func (l *FaktoryLogger) Fatalf(format string, v ...interface{}) {
	message := fmt.Sprintf(format, v...)
	l.logger.Error(message)
}
