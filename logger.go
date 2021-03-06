// Copyright 2013, Örjan Persson. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package logging implements a logging infrastructure for Go. It supports
// different logging backends like syslog, file and memory. Multiple backends
// can be utilized with different log levels per backend and logger.
package logging

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Redactor is an interface for types that may contain sensitive information
// (like passwords), which shouldn't be printed to the log. The idea was found
// in relog as part of the vitness project.
type Redactor interface {
	Redacted() interface{}
}

// Redact returns a string of * having the same length as s.
func Redact(s string) string {
	return strings.Repeat("*", len(s))
}

var (
	// Sequence number is incremented and utilized for all log records created.
	sequenceNo uint64

	// timeNow is a customizable for testing purposes.
	timeNow = time.Now

	// loggers stores Log objects by module name
	loggers SyncedLoggers
)

// SyncedLoggers represents a parallel by module Logger registrator
type SyncedLoggers struct {
	loggers map[string]Logger
	mu      sync.RWMutex
}

// Get returns a Logger object is has be registered, other wise, nil
func (this *SyncedLoggers) Get(module string) Logger {
	this.mu.RLock()
	defer this.mu.RUnlock()
	if this.loggers == nil {
		return nil
	}
	return this.loggers[module]
}

// GetOrCreate returns a Logger object is has be registered, other wise, creates and registry new object
func (this *SyncedLoggers) GetOrCreate(module string) (log Logger) {
	if log = this.Get(module); log == nil {
		this.mu.Lock()
		defer this.mu.Unlock()
		if this.loggers == nil {
			this.loggers = map[string]Logger{}
		}
		log = NewLogger(module)
		this.loggers[module] = log
	}
	return
}

var MustGetLogger = GetOrCreateLogger

// Record representslog static record and contains the timestamp when the record
// was created, an increasing id, filename and line and finally the actual
// formatted log line.
type RecordData struct {
	ID      uint64
	Time    time.Time
	Module  string
	Level   Level
	Message string
}

// Record represents a log record and contains the timestamp when the record
// was created, an increasing id, filename and line and finally the actual
// formatted log line.
type Record struct {
	ID     uint64
	Time   time.Time
	Module string
	Level  Level
	Args   []interface{}

	// message is kept as a pointer to have shallow copies update this once
	// needed.
	message   *string
	fmt       *string
	formatter Formatter
	formatted string
}

// Formatted returns the formatted log record string.
func (r *Record) Formatted(calldepth int) string {
	if r.formatted == "" {
		var buf bytes.Buffer
		r.formatter.Format(calldepth+1, r, &buf)
		r.formatted = buf.String()
	}
	return r.formatted
}

// Message returns the log record message.
func (r *Record) Message() string {
	if r.message == nil {
		// Redact the arguments that implements the Redactor interface
		for i, arg := range r.Args {
			if redactor, ok := arg.(Redactor); ok == true {
				r.Args[i] = redactor.Redacted()
			}
		}
		var buf bytes.Buffer
		if r.fmt != nil {
			fmt.Fprintf(&buf, *r.fmt, r.Args...)
		} else {
			// use Fprintln to make sure we always get space between arguments
			fmt.Fprintln(&buf, r.Args...)
			buf.Truncate(buf.Len() - 1) // strip newline
		}
		msg := buf.String()
		r.message = &msg
	}
	return *r.message
}

// Data returns the RecordData object.
func (r *Record) Data() RecordData {
	return RecordData{
		r.ID,
		r.Time,
		r.Module,
		r.Level,
		r.Message(),
	}
}

// Log is the actual logger which creates log records based on the functions
// called and passes them to the underlying logging backend.
type Log struct {
	Basic
	Module      string
	backend     LeveledBackend
	haveBackend bool
}

// NewLogger crates new Log object with module name
func NewLogger(module string) *Log {
	l := &Log{Module: module}
	l.writer = DefaultWriter(l, l.Module)
	return l
}

// SetBackend overrides any previously defined backend for this logger.
func (l *Log) SetBackend(backend LeveledBackend) {
	l.backend = backend
	l.haveBackend = true
}

// Backend return current backend if has be defined
func (l *Log) Backend() LeveledBackend {
	return l.backend
}

// IsEnabledFor returns true if the logger is enabled for the given level.
func (l *Log) IsEnabledFor(level Level) bool {
	return defaultBackend.IsEnabledFor(level, l.Module)
}

// GetOrCreateLogger returns a Logger object is has be registered in Loggers, other wise, creates and registry new object
func GetOrCreateLogger(module string) Logger {
	return loggers.GetOrCreate(module)
}

// GetLogger returns a Logger object based on the module name registered in Loggers.
func GetLogger(module string) Logger {
	return loggers.Get(module)
}

// MainLogger returns a Logger object based on the sys.Argv[0].
func MainLogger() Logger {
	return GetOrCreateLogger(filepath.Base(os.Args[0]))
}

// Reset restores the internal state of the logging library.
func Reset() {
	// TODO make a global Init() method to be less magic? or make it such that
	// if there's no backends at all configured, we could use some tricks to
	// automatically setup backends based if we have a TTY or not.
	sequenceNo = 0
	b := SetBackend(NewLogBackend(os.Stderr, "", log.LstdFlags))
	b.SetLevel(DEBUG, "")
	SetFormatter(DefaultFormatter)
	timeNow = time.Now
}

func init() {
	Reset()
}
