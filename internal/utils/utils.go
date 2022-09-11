package utils

import (
	"strings"

	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

func AreSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

var Log = logrus.New()

func SetLogLevel(level string) {
	// We are not using logrus' trace and panic levels
	switch strings.ToLower(level) {
	case "debug":
		Log.SetLevel(log.DebugLevel)
	case "info":
		Log.SetLevel(log.InfoLevel)
	case "warning":
		Log.SetLevel(log.WarnLevel)
	case "error":
		Log.SetLevel(log.ErrorLevel)
	case "fatal":
		Log.SetLevel(log.FatalLevel)
	default:
		log.Fatal("Bad error level string")
	}
}
