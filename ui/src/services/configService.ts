import { getSecret } from "src/api/secrets/secrets"

export interface AnalyticsConfig {
  amplitude?: {
    apiKey: string
  }
  bugsnag?: {
    apiKey: string
    appVersion?: string
  }
  releaseStage?: string
}

export class ConfigService {
  private static readonly SECRET_NAME = "analytics-keys"
  private static readonly SECRET_NAMESPACE = "migration-system"

  static async fetchAnalyticsConfig(): Promise<AnalyticsConfig | null> {
    try {
      const secret = await getSecret(this.SECRET_NAME, this.SECRET_NAMESPACE)

      const data = secret.data || {}

      const config: AnalyticsConfig = {}

      // Extract shared release stage
      if (data["release-stage"]) {
        config.releaseStage = data["release-stage"]
      }

      // Extract Amplitude config
      if (data["amplitude-api-key"]) {
        config.amplitude = {
          apiKey: data["amplitude-api-key"],
        }
      }

      // Extract Bugsnag config
      if (data["bugsnag-api-key"]) {
        config.bugsnag = {
          apiKey: data["bugsnag-api-key"],
          appVersion: data["app-version"] || undefined,
        }
      }

      return config
    } catch (error) {
      console.error(
        "Failed to fetch analytics configuration from Secret:",
        error
      )
      return null
    }
  }
}
