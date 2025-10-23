package scrapfly

import (
	"log"
	"os"
)

// LogLevel defines the level of logging.
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Logger is a simple logger with levels.
type Logger struct {
	logger *log.Logger
	level  LogLevel
}

// NewLogger creates a new Logger instance.
func NewLogger(name string) *Logger {
	return &Logger{
		logger: log.New(os.Stdout, name+": ", log.LstdFlags),
		level:  LevelInfo,
	}
}

// SetLevel sets the logging level.
func (l *Logger) SetLevel(level LogLevel) {
	l.level = level
}

func (l *Logger) Debug(v ...interface{}) {
	if l.level <= LevelDebug {
		l.logger.Println(append([]interface{}{"[DEBUG]"}, v...)...)
	}
}

func (l *Logger) Info(v ...interface{}) {
	if l.level <= LevelInfo {
		l.logger.Println(append([]interface{}{"[INFO]"}, v...)...)
	}
}

func (l *Logger) Warn(v ...interface{}) {
	if l.level <= LevelWarn {
		l.logger.Println(append([]interface{}{"[WARN]"}, v...)...)
	}
}

func (l *Logger) Error(v ...interface{}) {
	if l.level <= LevelError {
		l.logger.Println(append([]interface{}{"[ERROR]"}, v...)...)
	}
}

// Logger is the default logger for the scrapefly package.
var DefaultLogger = NewLogger("scrapefly")
