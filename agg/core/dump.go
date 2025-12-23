package core

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func DumpErrorLog(name, msg string) {
	baseDir, err := os.UserHomeDir()
	if err != nil {
		baseDir = "."
	}
	baseDir = filepath.Join(baseDir, errorLogsDir)

	os.MkdirAll(baseDir, 0755)
	filename := filepath.Join(baseDir, fmt.Sprintf("%s-%s.log", name, time.Now().Format("20060102150405")))
	os.WriteFile(filename, []byte(msg), 0644)
}

const errorLogsDir = "/.opa/error_logs"
