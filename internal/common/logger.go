package common

import (
	"fmt"
	"log"
)

type Logger interface {
	Debug(id, format string, v ...any)
}

func NewLogger(messagePrefix string) Logger {
	return &logger{
		messagePrefix: messagePrefix,
	}
}

type logger struct {
	messagePrefix string
}

// Debug implements Logger.
func (l *logger) Debug(id, format string, v ...any) {
	idPart := ""
	if id != "" {
		idPart = fmt.Sprintf(" - %s", id)
	}
	log.Printf("%s%s - %s", l.messagePrefix, idPart, fmt.Sprintf(format, v...))
}
