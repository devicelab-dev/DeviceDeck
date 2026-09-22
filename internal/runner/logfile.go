package runner

import "github.com/devicelab-dev/maestro-runner/pkg/logger"

// SetLogFile sends the maestro-runner driver's own diagnostics (driver
// install and start, retries, element lookups) to path. Without it the
// runner drops every one of them, because its logger writes nothing until
// it is given a file. Kept here so the rest of DeviceDeck never imports the
// runner's logger directly.
func SetLogFile(path string) error { return logger.Init(path) }

// CloseLogFile flushes and closes the driver log opened by SetLogFile.
func CloseLogFile() { logger.Close() }
