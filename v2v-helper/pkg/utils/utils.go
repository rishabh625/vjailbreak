package utils

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"
	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	migrationconstants "github.com/platform9/vjailbreak/k8s/migration/pkg/constants"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/constants"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
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

func ConvertToK8sName(name string) (string, error) {
	// Convert to lowercase
	name = strings.ToLower(name)
	// Replace separators with hyphens
	re := regexp.MustCompile(`[_\s]`)
	name = re.ReplaceAllString(name, "-")
	// Remove all characters that are not lowercase alphanumeric, hyphens, or periods
	re = regexp.MustCompile(`[^a-z0-9\-.]`)
	name = re.ReplaceAllString(name, "")
	// Remove leading and trailing hyphens
	name = strings.Trim(name, "-")
	// Truncate to 242 characters, as we prepend v2v-helper- to the name
	if len(name) > migrationconstants.NameMaxLength {
		name = name[:migrationconstants.NameMaxLength]
	}
	nameerrors := validation.IsQualifiedName(name)
	if len(nameerrors) == 0 {
		return name, nil
	}
	return name, fmt.Errorf("name '%s' is not a valid K8s name: %v", name, nameerrors)
}

func IsDebug(ctx context.Context, client client.Client) (bool, error) {
	// get the configmap
	configMapName, err := GetMigrationConfigMapName(os.Getenv("SOURCE_VM_NAME"))
	if err != nil {
		return false, err
	}
	configMap := &v1.ConfigMap{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      configMapName,
		Namespace: constants.MigrationSystemNamespace,
	}, configMap)
	if err != nil {
		return false, errors.Wrap(err, "Failed to get configmap")
	}
	debug := strings.TrimSpace(string(configMap.Data["DEBUG"]))
	return debug == constants.TrueString, nil
}

func PrintLog(logMessage string) error {
	log.Println(logMessage)
	return WriteToLogFile(logMessage)
}

func GetMigrationObjectName() (string, error) {
	vmname := os.Getenv("SOURCE_VM_NAME")
	vmK8sName, err := ConvertToK8sName(vmname)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("migration-%s", vmK8sName), nil
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
