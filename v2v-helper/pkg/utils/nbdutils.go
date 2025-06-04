package utils

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/platform9/vjailbreak/v2v-helper/pkg/constants"
)

func ParseFraction(text string) (int, int, error) {
	var numerator, denominator int
	_, err := fmt.Sscanf(text, "%d/%d", &numerator, &denominator)
	if err != nil {
		return 0, 0, err
	}
	return numerator, denominator, nil
}

func AddDebugOutputToFile(cmd *exec.Cmd) {
	migrationName, err := GetMigrationObjectName()
	if err == nil {
		// Ensure logs directory exists
		if err := os.MkdirAll(constants.LogsDir, 0755); err == nil {
			// Create log file with the migration object name
			logFilePath := fmt.Sprintf("%s/%s.log", constants.LogsDir, migrationName)
			logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if err == nil {
				PrintLog(fmt.Sprintf("Debug mode enabled. Command output will be logged to %s", logFilePath))

				// Set stdout/stderr to only write to the log file, not to kubectl logs
				cmd.Stdout = logFile
				cmd.Stderr = logFile
			}
		}
	}
}
