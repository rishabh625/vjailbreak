import {
  deleteMigrationPlan,
  getMigrationPlansList,
} from "./migration-plans/migrationPlans"
import {
  deleteMigrationTemplate,
  getMigrationTemplatesList,
} from "./migration-templates/migrationTemplates"
import {
  deleteNetworkMapping,
  getNetworkMappingList,
} from "./network-mapping/networkMappings"
import {
  deleteOpenstackCredentials,
  getOpenstackCredentialsList,
} from "./openstack-creds/openstackCreds"
import {
  deleteStorageMapping,
  getStorageMappingsList,
} from "./storage-mappings/storageMappings"
import {
  deleteVmwareCredentials,
  getVmwareCredentialsList,
} from "./vmware-creds/vmwareCreds"
import {
  createOpenstackCredsSecret,
  createVMwareCredsSecret,
  deleteSecret,
} from "./secrets/secrets"
import { createOpenstackCredsWithSecret } from "./openstack-creds/openstackCreds"
import { createVMwareCredsWithSecret } from "./vmware-creds/vmwareCreds"
import { VJAILBREAK_DEFAULT_NAMESPACE } from "./constants"

export const cleanupAllResources = async () => {
  // Clean up vmware creds
  try {
    const vmwareCreds = await getVmwareCredentialsList()
    for (const vmwareCred of vmwareCreds) {
      await deleteVmwareCredentials(vmwareCred.metadata.name)
    }
  } catch (e) {
    console.error("Error cleaning up vmware creds", e)
  }

  // Clean up openstack creds
  try {
    const openstackCreds = await getOpenstackCredentialsList()
    for (const openstackCred of openstackCreds) {
      await deleteOpenstackCredentials(openstackCred.metadata.name)
    }
  } catch (e) {
    console.error("Error cleaning up openstack creds", e)
  }

  // Clean up network mappings
  try {
    const networkMappings = await getNetworkMappingList()
    for (const networkMapping of networkMappings) {
      await deleteNetworkMapping(networkMapping.metadata.name)
    }
  } catch (e) {
    console.error("Error cleaning up network mappings", e)
  }

  // Clean up storage mappings
  try {
    const storageMappings = await getStorageMappingsList()
    for (const storageMapping of storageMappings) {
      await deleteStorageMapping(storageMapping.metadata.name)
    }
  } catch (e) {
    console.error("Error cleaning up storage mappings", e)
  }

  // Clean up migration templates
  try {
    const migrationTemplates = await getMigrationTemplatesList()
    for (const migrationTemplate of migrationTemplates) {
      await deleteMigrationTemplate(migrationTemplate.metadata.name)
    }
  } catch (e) {
    console.error("Error cleaning up migration templates", e)
  }

  // Clean up migration plans. This will also clean up migrations
  try {
    const migrationPlans = await getMigrationPlansList()
    for (const migrationPlan of migrationPlans) {
      await deleteMigrationPlan(migrationPlan.metadata.name)
    }
  } catch (e) {
    console.error("Error cleaning up migration plans", e)
  }
}

// Create OpenStack credentials with secret
export const createOpenstackCredsWithSecretFlow = async (
  credName: string,
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
  const secretName = `${credName}-openstack-secret`

  await createOpenstackCredsSecret(secretName, credentials, namespace)

  return createOpenstackCredsWithSecret(credName, secretName, namespace)
}

// Create VMware credentials with secret
export const createVMwareCredsWithSecretFlow = async (
  credName: string,
  credentials,
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  const secretName = `${credName}-vmware-secret`

  await createVMwareCredsSecret(secretName, credentials, namespace)

  return createVMwareCredsWithSecret(
    credName,
    secretName,
    namespace,
    credentials.VCENTER_DATACENTER
  )
}

export const deleteVMwareCredsWithSecretFlow = async (
  credName: string,
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  try {
    const secretName = `${credName}-vmware-secret`
    await deleteVmwareCredentials(credName, namespace)
    await deleteSecret(secretName, namespace)
    return { success: true }
  } catch (error) {
    console.error(`Error deleting VMware credential ${credName}:`, error)
    throw error
  }
}

// Delete OpenStack credentials and associated secret
export const deleteOpenStackCredsWithSecretFlow = async (
  credName: string,
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  try {
    const secretName = `${credName}-openstack-secret`
    await deleteOpenstackCredentials(credName, namespace)
    await deleteSecret(secretName, namespace)
    return { success: true }
  } catch (error) {
    console.error(`Error deleting OpenStack credential ${credName}:`, error)
    throw error
  }
}
