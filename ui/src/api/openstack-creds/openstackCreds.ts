import axios from "../axios"

import {
  VJAILBREAK_API_BASE_PATH,
  VJAILBREAK_DEFAULT_NAMESPACE,
} from "../constants"
import { GetOpenstackCredsList, OpenstackCreds } from "./model"

export const getOpenstackCredentialsList = async (
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  const endpoint = `${VJAILBREAK_API_BASE_PATH}/namespaces/${namespace}/openstackcreds`
  const response = await axios.get<GetOpenstackCredsList>({
    endpoint,
  })
  return response?.items
}

export const getOpenstackCredentials = async (
  name,
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  const endpoint = `${VJAILBREAK_API_BASE_PATH}/namespaces/${namespace}/openstackcreds/${name}`
  const response = await axios.get<OpenstackCreds>({
    endpoint,
  })
  return response
}

export const postOpenstackCredentials = async (
  data,
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  const endpoint = `${VJAILBREAK_API_BASE_PATH}/namespaces/${namespace}/openstackcreds`
  const response = await axios.post<OpenstackCreds>({
    endpoint,
    data,
  })
  return response
}

export const deleteOpenstackCredentials = async (
  name,
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  const endpoint = `${VJAILBREAK_API_BASE_PATH}/namespaces/${namespace}/openstackcreds/${name}`
  const response = await axios.del<OpenstackCreds>({
    endpoint,
  })
  return response
}

// Create OpenStack credentials with secret reference
export const createOpenstackCredsWithSecret = async (
  name: string,
  secretName: string,
  namespace = VJAILBREAK_DEFAULT_NAMESPACE
) => {
  const endpoint = `${VJAILBREAK_API_BASE_PATH}/namespaces/${namespace}/openstackcreds`

  const credBody = {
    apiVersion: "vjailbreak.k8s.pf9.io/v1alpha1",
    kind: "OpenstackCreds",
    metadata: {
      name,
      namespace,
    },
    spec: {
      secretRef: {
        name: secretName,
      },
    },
  }

  const response = await axios.post<OpenstackCreds>({
    endpoint,
    data: credBody,
  })

  return response
}

// IP validation interfaces and function
interface IPValidationRequest {
  ip: string[]
  accessInfo: {
    authUrl: string
    domainName: string
    insecure: boolean
    password: string
    regionName: string
    tenantName: string
    username: string
  }
}

interface IPValidationResponse {
  isValid: boolean[]
  reason: string[]
}

export const validateOpenstackIPs = async (
  request: IPValidationRequest
): Promise<IPValidationResponse> => {
  const endpoint = "/dev-api/sdk/vpw/v1/validate_openstack_ip"
  const response = await axios.post<IPValidationResponse>({
    endpoint,
    data: request,
  })

  return response
}
