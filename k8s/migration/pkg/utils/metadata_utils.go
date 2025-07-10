package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/pkg/errors"
)

// InstanceMetadata contains metadata about the current instance.
type InstanceMetadata struct {
	UUID string `json:"uuid"`
}

// GetCurrentInstanceMetadata retrieves metadata about the current instance from the OpenStack metadata service
func GetCurrentInstanceMetadata() (*InstanceMetadata, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", "http://169.254.169.254/openstack/latest/meta_data.json", nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create metadata request")
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch instance metadata")
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("[WARN] Failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from metadata service: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read metadata response")
	}

	var metadata InstanceMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, errors.Wrap(err, "failed to parse metadata")
	}

	if metadata.UUID == "" {
		return nil, errors.New("instance UUID not found in metadata")
	}

	return &metadata, nil
}

// VerifyCredentialsMatchCurrentEnvironment checks if the provided credentials can access the current instance
func VerifyCredentialsMatchCurrentEnvironment(providerClient *gophercloud.ProviderClient) (bool, error) {
	// Get current instance metadata
	metadata, err := GetCurrentInstanceMetadata()
	if err != nil {
		return false, fmt.Errorf("unable to get current instance metadata: %w. "+
			"Please ensure this is running on an OpenStack instance with metadata service enabled", err)
	}

	computeClient, err := openstack.NewComputeV2(providerClient, gophercloud.EndpointOpts{})
	if err != nil {
		return false, fmt.Errorf("failed to create OpenStack compute client: %w", err)
	}
	_, err = servers.Get(computeClient, metadata.UUID).Extract()
	if err != nil {
		if strings.Contains(err.Error(), "Resource not found") ||
			strings.Contains(err.Error(), "No server with a name or ID") {
			return false, errors.New("Creds are valid but for a different OpenStack environment. Enter creds of same OpenStack environment")
		}
		return false, fmt.Errorf("failed to verify instance access: %w. "+
			"Please check if the provided credentials have compute:get_server permission", err)
	}
	return true, nil
}
