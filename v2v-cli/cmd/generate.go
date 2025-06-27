package cmd

import (
	"bufio"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v2"
)

const podYamlTemplate = `apiVersion: v1
kind: Pod
metadata:
  name: v2vhelper
  labels:
    vm-name: mig_vm_name
spec:
  restartPolicy: Never
  containers:
  - name: fedora
    securityContext:
      privileged: true
    image: platform9/v2v-helper:v0.1
    imagePullPolicy: Always
    command:
    - /home/fedora/manager
    envFrom:
    - configMapRef:
        name: migration-config
    volumeMounts:
    - mountPath: /home/fedora/vmware-vix-disklib-distrib
      name: vddk
    - mountPath: /dev
      name: dev
  volumes:
  - name: vddk
    hostPath:
      path: /home/ubuntu/vmware-vix-disklib-distrib
      type: Directory
  - name: dev
    hostPath:
      path: /dev
      type: Directory
`

var (
	VCENTER_USERNAME      string
	VCENTER_PASSWORD      string
	VCENTER_HOST          string
	SOURCE_VM_NAME        string
	CONVERT               string
	VCENTER_INSECURE      string
	NEUTRON_NETWORK_NAMES string
)

type ConfigMap struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Data map[string]string `yaml:"data"`
}

func randSeq(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyz")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func readBool(value string) string {
	if value == "" || strings.ToLower(value) == "true" || strings.ToLower(value) == "t" {
		return "true"
	} else if strings.ToLower(value) == "false" || strings.ToLower(value) == "f" {
		return "false"
	}
	return "true"
}

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Generate Kubernetes Pod and ConfigMap YAML",
	Long: `Generate Kubernetes Pod and ConfigMap YAML based on user input and admin.rc file and start the migration. 
Your admin.rc file should atleast contain the following keys: OS_AUTH_URL, OS_DOMAIN_NAME, OS_TENANT_NAME, OS_USERNAME, OS_PASSWORD.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Check if kubectl exists
		_, err := exec.LookPath("kubectl")
		if err != nil {
			log.Fatal("kubectl not found. Please make sure it is installed and in your PATH.")
		}
		// Create the ConfigMap structure
		var configMap ConfigMap
		configMap.APIVersion = "v1"
		configMap.Kind = "ConfigMap"
		configMap.Metadata.Name = "migration-config"
		configMap.Data = make(map[string]string)

		envVars := []string{"VCENTER_USERNAME", "VCENTER_PASSWORD", "VCENTER_HOST", "SOURCE_VM_NAME", "CONVERT", "VCENTER_INSECURE", "NEUTRON_NETWORK_NAMES", "OS_FAMILY", "VIRTIO_WIN_DRIVER"}
		for _, env := range envVars {
			var value string
			switch env {
			case "VCENTER_USERNAME":
				if vcenter_username, _ := cmd.Flags().GetString("vcenter-user"); vcenter_username != "" {
					value = vcenter_username
				} else {
					fmt.Printf("Enter value for %s: ", env)
					fmt.Scanln(&value)
				}
			case "VCENTER_PASSWORD":
				if vcenter_password, _ := cmd.Flags().GetString("vcenter-password"); vcenter_password != "" {
					value = vcenter_password
				} else {
					fmt.Printf("Enter value for %s: ", env)
					password, _ := term.ReadPassword(0)
					value = string(password)
					fmt.Println()
				}
			case "VCENTER_HOST":
				if vcenter_host, _ := cmd.Flags().GetString("vcenter-host"); vcenter_host != "" {
					value = vcenter_host
				} else {
					fmt.Printf("Enter value for %s: ", env)
					fmt.Scanln(&value)
				}
			case "SOURCE_VM_NAME":
				if source_vm_name, _ := cmd.Flags().GetString("source-vm-name"); source_vm_name != "" {
					value = source_vm_name
				} else {
					fmt.Printf("Enter value for %s: ", env)
					fmt.Scanln(&value)
				}
			case "VCENTER_INSECURE":
				if vcenter_insecure, _ := cmd.Flags().GetString("vcenter-insecure"); vcenter_insecure != "" {
					value = vcenter_insecure
				} else {
					fmt.Printf("Enter value for %s (true/false) (Default is true):", env)
					fmt.Scanln(&value)
					value = readBool(value)
				}
			case "CONVERT":
				if convert, _ := cmd.Flags().GetString("convert"); convert != "" {
					value = convert
				} else {
					fmt.Printf("Enter value for %s (true/false) (Default is true):", env)
					fmt.Scanln(&value)
					value = readBool(value)
				}
			case "OS_FAMILY":
				if os_type, _ := cmd.Flags().GetString("os-type"); os_type != "" {
					value = os_type
				} else {
					fmt.Printf("Enter value for %s (Windows/Linux): ", env)
					fmt.Scanln(&value)
				}
			case "NEUTRON_NETWORK_NAMES":
				if neutron_network_name, _ := cmd.Flags().GetString("neutron-network-name"); neutron_network_name != "" {
					value = neutron_network_name
				} else {
					fmt.Printf("Enter value for %s (Default is vlan-218-uservm-network-1): ", env)
					reader := bufio.NewReader(os.Stdin)
					value, _ = reader.ReadString('\n')
					value = strings.TrimSuffix(value, "\n")
					if value == "" {
						value = "vlan-218-uservm-network-1"
					}
				}
			case "VIRTIO_WIN_DRIVER":
				if virtio_win_driver, _ := cmd.Flags().GetString("virtio-win-iso"); virtio_win_driver != "" {
					value = virtio_win_driver
				} else {
					fmt.Printf("Enter value for %s: ", env)
					fmt.Scanln(&value)
				}
			default:
				fmt.Printf("Enter value for %s: ", env)
				fmt.Scanln(&value)
			}
			configMap.Data[env] = value
		}

		randsequence := randSeq(5)

		configMap.Metadata.Name = configMap.Metadata.Name + "-" + randsequence

		var adminFile string
		if admin_rc, _ := cmd.Flags().GetString("admin-file"); admin_rc != "" {
			adminFile = admin_rc
		} else {
			adminFile = "admin.rc"
		}
		adminConfig, err := parseAdminFile(adminFile)
		if err != nil {
			log.Fatalf("Error reading admin file: %v", err)
		}

		openStackEnvVars := []string{"OS_AUTH_URL", "OS_DOMAIN_NAME", "OS_TENANT_NAME", "OS_USERNAME", "OS_PASSWORD", "OS_REGION_NAME"}
		for _, env := range openStackEnvVars {
			value, ok := adminConfig[env]
			if !ok {
				log.Fatalf("Missing key %s in admin file", env)
			}
			configMap.Data[env] = value
		}

		// Marshal and write the ConfigMap YAML
		configMapYamlData, err := yaml.Marshal(&configMap)
		if err != nil {
			log.Fatalf("Error marshalling ConfigMap to YAML: %v", err)
		}

		configMapOutputFile := "configmap.yaml"
		if err := os.WriteFile(configMapOutputFile, configMapYamlData, 0644); err != nil {
			log.Fatalf("Error writing ConfigMap YAML file: %v", err)
		}

		// Update the Pod YAML template with the correct ConfigMap name
		podYaml := podYamlTemplate
		podYaml = strings.ReplaceAll(podYaml, "migration-config", configMap.Metadata.Name)
		podYaml = strings.ReplaceAll(podYaml, "v2vhelper", "v2v-helper"+"-"+randsequence)
		podYaml = strings.ReplaceAll(podYaml, "mig_vm_name", configMap.Data["SOURCE_VM_NAME"])

		// Write the Pod YAML to a file
		podOutputFile := "pod.yaml"
		if err := os.WriteFile(podOutputFile, []byte(podYaml), 0644); err != nil {
			log.Fatalf("Error writing Pod YAML file: %v", err)
		}

		fmt.Printf("YAML files created successfully: %s, %s\n", podOutputFile, configMapOutputFile)

		// Apply the generated YAML files using kubectl
		applyCmd := exec.Command("kubectl", "apply", "-f", "configmap.yaml", "-f", "pod.yaml")
		applyCmd.Stdout = os.Stdout
		applyCmd.Stderr = os.Stderr
		if err := applyCmd.Run(); err != nil {
			log.Fatalf("Error applying YAML files: %v", err)
		}
		fmt.Println("YAML files applied successfully.")
		fmt.Printf("To check the status of the migration, run: kubectl logs -f v2v-helper-%s\n", randsequence)
	},
}

func parseAdminFile(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	envVars := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "export ") {
			parts := strings.SplitN(line[7:], "=", 2)
			if len(parts) == 2 {
				key := parts[0]
				value := strings.Trim(parts[1], "\"")
				envVars[key] = value
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return envVars, nil
}

var statusCmd = &cobra.Command{
	Use:   "status [vm-name]",
	Short: "Check the status of a migration",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		vmName := args[0]
		// Check if kubectl exists
		_, err := exec.LookPath("kubectl")
		if err != nil {
			log.Fatal("kubectl not found. Please make sure it is installed and in your PATH.")
		}
		// Get the pod with the matching label
		getPodCmd := exec.Command("kubectl", "get", "pod", "-l", "vm-name="+vmName, "-o", "jsonpath={.items[0].metadata.name}")
		podNameBytes, err := getPodCmd.Output()
		if err != nil {
			log.Fatalf("Error getting pod name: %v", err)
		}
		podName := strings.TrimSpace(string(podNameBytes))
		if podName == "" {
			log.Fatalf("No pod found with the vm-name label: %s", vmName)
		}
		// Get the pod logs
		logsCmd := exec.Command("kubectl", "logs", "-f", podName)
		logsCmd.Stdout = os.Stdout
		logsCmd.Stderr = os.Stderr
		if err := logsCmd.Run(); err != nil {
			log.Fatalf("Error getting pod logs: %v", err)
		}
	},
}
