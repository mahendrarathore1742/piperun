// Package logger provides coloured, per-stage prefixed log output.
package logger

import (
	"fmt"
	"sync"
	"time"
)

// ANSI colour codes
const (
	Reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Blue   = "\033[34m"
	Cyan   = "\033[36m"
	Bold   = "\033[1m"
)

// colours assigned round-robin to stages
var palette = []string{Green, Blue, Cyan, Yellow}

// Logger is the central logger shared across the engine.
type Logger struct {
	mu       sync.Mutex
	start    time.Time
	colourID int
	colours  map[string]string
}

// New creates a new Logger.
func New() *Logger {
	return &Logger{
		start:   time.Now(),
		colours: make(map[string]string),
	}
}

func (l *Logger) colourFor(stage string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if c, ok := l.colours[stage]; ok {
		return c
	}
	c := palette[l.colourID%len(palette)]
	l.colourID++
	l.colours[stage] = c
	return c
}

func (l *Logger) elapsed() string {
	d := time.Since(l.start)
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// Info logs an info-level message for a given stage.
func (l *Logger) Info(stage, msg string) {
	c := l.colourFor(stage)
	fmt.Printf("%s%s • %s[stage.%s]%s  %s\n", Bold, l.elapsed(), c, stage, Reset, msg)
}

// Warn logs a warning-level message.
func (l *Logger) Warn(stage, msg string) {
	c := l.colourFor(stage)
	fmt.Printf("%s%s • %s[stage.%s]%s  %s%s%s\n", Bold, l.elapsed(), c, stage, Reset, Yellow, msg, Reset)
}

// Error logs an error-level message.
func (l *Logger) Error(stage, msg string) {
	fmt.Printf("%s%s • %s[stage.%s]%s  %s%s%s\n", Bold, l.elapsed(), Red, stage, Reset, Red, msg, Reset)
}

// System logs a system-level (non-stage) message.
func (l *Logger) System(msg string) {
	fmt.Printf("%s%s • %s%s\n", Bold, l.elapsed(), Reset, msg)
}

// Done prints the final elapsed time.
func (l *Logger) Done() {
	fmt.Printf("%s%s • took %s%s\n", Bold, l.elapsed(), l.elapsed(), Reset)
}
