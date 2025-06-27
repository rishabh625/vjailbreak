import axios from "../axios"
import { VJAILBREAK_DEFAULT_NAMESPACE } from "../constants"
import { Secret } from "./model"

// Interface for secret data
export interface SecretData {
  [key: string]: string
}

// Function to create a Kubernetes secret
export const createSecret = async (
  name: string,
  data: SecretData,
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  // Convert data values to base64
  const base64Data = Object.entries(data).reduce((acc, [key, value]) => {
    acc[key] = btoa(value)
    return acc
  }, {} as SecretData)

  const secretBody = {
    apiVersion: "v1",
    kind: "Secret",
    metadata: {
      name,
      namespace,
    },
    type: "Opaque",
    data: base64Data,
  }

  // Use the Kubernetes API endpoint for secrets
  const endpoint = `/api/v1/namespaces/${namespace}/secrets`

  const response = await axios.post({
    endpoint,
    data: secretBody,
  })

  return response
}

// Function to create OpenStack credentials secret
export const createOpenstackCredsSecret = async (
  name: string,
  credentials: {
    OS_USERNAME: string
    OS_PASSWORD: string
    OS_AUTH_URL: string
    OS_PROJECT_NAME?: string
    OS_TENANT_NAME?: string
    OS_DOMAIN_NAME: string
    OS_REGION_NAME?: string
    OS_INSECURE?: boolean
  },
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  // Prepare data for the secret
  const secretData: SecretData = {
    OS_USERNAME: credentials.OS_USERNAME,
    OS_PASSWORD: credentials.OS_PASSWORD,
    OS_AUTH_URL: credentials.OS_AUTH_URL,
    OS_DOMAIN_NAME: credentials.OS_DOMAIN_NAME,
  }

  // Add optional fields if they exist
  if (credentials.OS_PROJECT_NAME) {
    secretData.OS_PROJECT_NAME = credentials.OS_PROJECT_NAME
  }

  if (credentials.OS_TENANT_NAME) {
    secretData.OS_TENANT_NAME = credentials.OS_TENANT_NAME
  }

  if (credentials.OS_REGION_NAME) {
    secretData.OS_REGION_NAME = credentials.OS_REGION_NAME
  }

  // Add OS_INSECURE if provided
  if (credentials.OS_INSECURE !== undefined) {
    secretData.OS_INSECURE = credentials.OS_INSECURE.toString()
  }

  return createSecret(name, secretData, namespace)
}

// Function to create VMware credentials secret
export const createVMwareCredsSecret = async (
  name: string,
  credentials: {
    VCENTER_HOST: string
    VCENTER_USERNAME: string
    VCENTER_PASSWORD: string
    VCENTER_DATACENTER: string
    VCENTER_INSECURE: boolean
  },
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  let vcenterHost = credentials.VCENTER_HOST
  if (vcenterHost.toLowerCase().startsWith("https://")) {
    vcenterHost = vcenterHost.substring(8) // Remove 'https://'
  } else if (vcenterHost.toLowerCase().startsWith("http://")) {
    vcenterHost = vcenterHost.substring(7) // Remove 'http://'
  }

  // Prepare data for the secret
  const secretData: SecretData = {
    VCENTER_HOST: vcenterHost,
    VCENTER_USERNAME: credentials.VCENTER_USERNAME,
    VCENTER_PASSWORD: credentials.VCENTER_PASSWORD,
    VCENTER_DATACENTER: credentials.VCENTER_DATACENTER,
    VCENTER_INSECURE: credentials.VCENTER_INSECURE ? "true" : "false",
  }

  return createSecret(name, secretData, namespace)
}

// Function to create BMConfig user-data secret
export const createBmconfigSecret = async (
  name: string,
  cloudInit: string,
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  // Prepare data for the secret
  const secretData: SecretData = {
    "user-data": cloudInit,
  }

  return createSecret(name, secretData, namespace)
}

// Function to get a Kubernetes secret
export const getSecret = async (
  name: string,
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
): Promise<Secret> => {
  const endpoint = `/api/v1/namespaces/${namespace}/secrets/${name}`

  try {
    const response: Secret = await axios.get({
      endpoint,
    })

    // If the secret has base64 encoded data, decode it
    if (response?.data) {
      // Kubernetes secrets data is in response.data.data
      const secretData = response.data || {}

      // Decode the base64 encoded values
      const decodedData = Object.entries(secretData).reduce(
        (acc, [key, value]) => {
          try {
            // Try to decode base64 values
            acc[key] = typeof value === "string" ? atob(value) : value
          } catch (error) {
            console.error(`Error decoding secret data for key ${key}:`, error)
            // If it's not base64 encoded, use the original value
            acc[key] = value
          }
          return acc
        },
        {} as SecretData
      )

      response.data = decodedData

      return response
    }

    return response
  } catch (error) {
    console.error(`Error getting secret ${name}:`, error)
    throw error
  }
}

export const deleteSecret = async (
  name: string,
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  const endpoint = `/api/v1/namespaces/${namespace}/secrets/${name}`
  const response = await axios.del({ endpoint })
  return response
}

export const getSecrets = async (namespace = VJAILBREAK_DEFAULT_NAMESPACE) => {
  const endpoint = `/api/v1/namespaces/${namespace}/secrets`
  const response = await axios.get({ endpoint })
  return response
}
