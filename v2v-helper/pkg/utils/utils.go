package utils

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/pkg/errors"
	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/constants"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	client "sigs.k8s.io/controller-runtime/pkg/client"
)

func RemoveEmptyStrings(slice []string) []string {
	var result []string
	for _, str := range slice {
		if str != "" {
			result = append(result, str)
		}
	}
	return result
}

func GetInclusterClient() (client.Client, error) {
	// Create a direct Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get in-cluster config")
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(vjailbreakv1alpha1.AddToScheme(scheme))
	clientset, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get in-cluster config")
	}

	return clientset, err
}

func PrintLog(logMessage string) error {
	log.Println(logMessage)
	return WriteToLogFile(logMessage)
}

func GetMigrationObjectName() (string, error) {
	vmK8sName, err := GetVMwareMachineName()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("migration-%s", vmK8sName), nil
}

// GetMigrationConfigMapName is function that returns the name of the secret
func GetMigrationConfigMapName() (string, error) {
	vmK8sName, err := GetVMwareMachineName()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("migration-config-%s", vmK8sName), nil
}

func GetVMwareMachineName() (string, error) {
	vmK8sName := os.Getenv("VMWARE_MACHINE_OBJECT_NAME")
	if vmK8sName == "" {
		return "", errors.New("VMWARE_MACHINE_OBJECT_NAME environment variable is not set")
	}
	return vmK8sName, nil
}

func WriteToLogFile(message string) error {
	// Get migration object name from environment variable
	migrationName, err := GetMigrationObjectName()
	if err != nil {
		return errors.Wrap(err, "failed to get migration object name")
	}

	// Ensure the logs directory exists
	if err := os.MkdirAll(constants.LogsDir, 0755); err != nil {
		return errors.Wrap(err, "failed to create logs directory")
	}

	// Create log file with the migration object name
	logFilePath := fmt.Sprintf("%s/%s.log", constants.LogsDir, migrationName)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to create log file")
	}
	defer logFile.Close()

	logMessage := fmt.Sprintf("[%s] %s\n", time.Now().Format(time.RFC3339), message)

	// Write the log message
	if _, err := logFile.WriteString(logMessage); err != nil {
		return errors.Wrap(err, "failed to write log message to log file")
	}
	return nil
}

func atoi(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return i
}

func GetVjailbreakSettings(ctx context.Context, k8sClient client.Client) (*VjailbreakSettings, error) {
	vjailbreakSettingsCM := &corev1.ConfigMap{}
	if err := k8sClient.Get(ctx, k8stypes.NamespacedName{Name: constants.VjailbreakSettingsConfigMapName, Namespace: constants.NamespaceMigrationSystem}, vjailbreakSettingsCM); err != nil {
		return nil, errors.Wrap(err, "failed to get vjailbreak settings configmap")
	}

	if vjailbreakSettingsCM.Data == nil {
		return &VjailbreakSettings{
			ChangedBlocksCopyIterationThreshold: constants.ChangedBlocksCopyIterationThreshold,
			VMActiveWaitIntervalSeconds:         constants.VMActiveWaitIntervalSeconds,
			VMActiveWaitRetryLimit:              constants.VMActiveWaitRetryLimit,
			DefaultMigrationMethod:              constants.DefaultMigrationMethod,
			VCenterScanConcurrencyLimit:         constants.VCenterScanConcurrencyLimit,
		}, nil
	}

	if vjailbreakSettingsCM.Data["CHANGED_BLOCKS_COPY_ITERATION_THRESHOLD"] == "" {
		vjailbreakSettingsCM.Data["CHANGED_BLOCKS_COPY_ITERATION_THRESHOLD"] = strconv.Itoa(constants.ChangedBlocksCopyIterationThreshold)
	}

	if vjailbreakSettingsCM.Data["VM_ACTIVE_WAIT_INTERVAL_SECONDS"] == "" {
		vjailbreakSettingsCM.Data["VM_ACTIVE_WAIT_INTERVAL_SECONDS"] = strconv.Itoa(constants.VMActiveWaitIntervalSeconds)
	}

	if vjailbreakSettingsCM.Data["VM_ACTIVE_WAIT_RETRY_LIMIT"] == "" {
		vjailbreakSettingsCM.Data["VM_ACTIVE_WAIT_RETRY_LIMIT"] = strconv.Itoa(constants.VMActiveWaitRetryLimit)
	}

	if vjailbreakSettingsCM.Data["DEFAULT_MIGRATION_METHOD"] == "" {
		vjailbreakSettingsCM.Data["DEFAULT_MIGRATION_METHOD"] = constants.DefaultMigrationMethod
	}

	if vjailbreakSettingsCM.Data["VCENTER_SCAN_CONCURRENCY_LIMIT"] == "" {
		vjailbreakSettingsCM.Data["VCENTER_SCAN_CONCURRENCY_LIMIT"] = strconv.Itoa(constants.VCenterScanConcurrencyLimit)
	}

	return &VjailbreakSettings{
		ChangedBlocksCopyIterationThreshold: atoi(vjailbreakSettingsCM.Data["CHANGED_BLOCKS_COPY_ITERATION_THRESHOLD"]),
		VMActiveWaitIntervalSeconds:         atoi(vjailbreakSettingsCM.Data["VM_ACTIVE_WAIT_INTERVAL_SECONDS"]),
		VMActiveWaitRetryLimit:              atoi(vjailbreakSettingsCM.Data["VM_ACTIVE_WAIT_RETRY_LIMIT"]),
		DefaultMigrationMethod:              vjailbreakSettingsCM.Data["DEFAULT_MIGRATION_METHOD"],
		VCenterScanConcurrencyLimit:         atoi(vjailbreakSettingsCM.Data["VCENTER_SCAN_CONCURRENCY_LIMIT"]),
	}, nil
}
