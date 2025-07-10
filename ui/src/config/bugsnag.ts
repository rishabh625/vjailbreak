export interface BugsnagConfig {
  apiKey: string
  appVersion: string
  releaseStage: string
  enabledReleaseStages: string[]
  collectUserIp: boolean
  autoDetectErrors: boolean
  autoTrackSessions: boolean
}

export interface BugsnagPerformanceConfig {
  apiKey: string
}

export const getBugsnagConfig = (): BugsnagConfig => {
  const isDevelopment = import.meta.env.MODE === "development"
  const isProduction = import.meta.env.MODE === "production"

  return {
    apiKey:
      import.meta.env.VITE_BUGSNAG_API_KEY ||
      "25ac6ab51e4d3f11b226f56008145003",
    appVersion: import.meta.env.VITE_APP_VERSION || "1.0.0",
    releaseStage: import.meta.env.MODE || "development",
    enabledReleaseStages: isProduction
      ? ["production"]
      : ["development", "staging", "production"],
    collectUserIp: false,
    autoDetectErrors: true,
    autoTrackSessions: !isDevelopment,
  }
}

export const getBugsnagPerformanceConfig = (): BugsnagPerformanceConfig => {
  return {
    apiKey:
      import.meta.env.VITE_BUGSNAG_API_KEY ||
      "25ac6ab51e4d3f11b226f56008145003",
  }
}
