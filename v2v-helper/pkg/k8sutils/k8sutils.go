package k8sutils

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	vjailbreakv1alpha1 "github.com/platform9/vjailbreak/k8s/migration/api/v1alpha1"
	"github.com/platform9/vjailbreak/v2v-helper/pkg/constants"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	client "sigs.k8s.io/controller-runtime/pkg/client"
)

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
	if len(name) > 242 {
		name = name[:242]
	}
	nameerrors := validation.IsQualifiedName(name)
	if len(nameerrors) == 0 {
		return name, nil
	}
	return name, fmt.Errorf("name '%s' is not a valid K8s name: %v", name, nameerrors)
}

func GetVMwareMachine(ctx context.Context, vmName string) (*vjailbreakv1alpha1.VMwareMachine, error) {
	client, err := GetInclusterClient()
	if err != nil {
		return nil, err
	}
	vmwareMachine := &vjailbreakv1alpha1.VMwareMachine{}
	vmK8sName, err := ConvertToK8sName(vmName)
	if err != nil {
		return nil, err
	}
	err = client.Get(ctx, types.NamespacedName{
		Name:      vmK8sName,
		Namespace: constants.MigrationSystemNamespace,
	}, vmwareMachine)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get vmware machine")
	}
	return vmwareMachine, nil
}
