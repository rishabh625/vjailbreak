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
			baseLogPath := fmt.Sprintf("%s/%s.log", constants.LogsDir, migrationName)
			logFilePath := baseLogPath
			
			// Check if file already exists, if so, create a new file with suffix
			if _, err := os.Stat(logFilePath); err == nil {
				// File exists, find an available suffix
				suffix := 1
				maxSuffix := 20 // Maximum suffix allowed
				
				for suffix <= maxSuffix {
					logFilePath = fmt.Sprintf("%s/%s.%d.log", constants.LogsDir, migrationName, suffix)
					if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
						// Found an available filename
						break
					}
					suffix++
				}
				
				// If we've reached the limit, overwrite the last numbered file
				if suffix > maxSuffix {
					// Use the max suffix (20) when the limit is reached
					logFilePath = fmt.Sprintf("%s/%s.%d.log", constants.LogsDir, migrationName, maxSuffix)
					// We'll overwrite this file
				}
			}
			
			// Create the log file (with a suffix if needed)
			logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				PrintLog(fmt.Sprintf("Debug mode enabled. Command output will be logged to %s", logFilePath))

				// Set stdout/stderr to only write to the log file, not to kubectl logs
				cmd.Stdout = logFile
				cmd.Stderr = logFile
			}
		}
	}
}
