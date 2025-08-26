package main

import (
	"os"
	"time"

	cblog "github.com/charmbracelet/log"
)

// Shared app logger for the web server package.
var logger = cblog.NewWithOptions(os.Stderr, cblog.Options{
	ReportTimestamp: true,
	TimeFormat:      time.RFC3339,
	ReportCaller:    false, // flip to true if you want file:line
})

// Call this once in main after reading config.
func configureLogger(logLevel string) {
	if logLevel == "debug" {
		logger.SetLevel(cblog.DebugLevel)
		logger.SetReportCaller(true)
	} else if logLevel == "info" {
		logger.SetLevel(cblog.InfoLevel)
	} else if logLevel == "warn" {
		logger.SetLevel(cblog.WarnLevel)
	} else {
		logger.SetLevel(cblog.ErrorLevel)
	}
}
