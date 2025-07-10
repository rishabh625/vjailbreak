import "@fontsource/roboto/300.css"
import "@fontsource/roboto/400.css"
import "@fontsource/roboto/500.css"
import "@fontsource/roboto/700.css"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { ReactQueryDevtools } from "@tanstack/react-query-devtools"
import React, { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter } from "react-router-dom"
import Bugsnag from '@bugsnag/js'
import BugsnagPluginReact from '@bugsnag/plugin-react'
import BugsnagPerformance from '@bugsnag/browser-performance'
import App from "./App.tsx"
import { ThemeProvider } from "./theme/ThemeContext.tsx"
import { getBugsnagConfig, getBugsnagPerformanceConfig } from "./config/bugsnag"
import { errorReportingService } from "./services/errorReporting"

const queryClient = new QueryClient()

const bugsnagConfig = getBugsnagConfig()
const bugsnagPerformanceConfig = getBugsnagPerformanceConfig()

if (bugsnagConfig.apiKey) {
  Bugsnag.start({
    ...bugsnagConfig,
    plugins: [new BugsnagPluginReact()],
  })

  BugsnagPerformance.start(bugsnagPerformanceConfig)

  errorReportingService.initialize(Bugsnag)

  errorReportingService.addMetadata('app', 'name', 'vjailbreak')
  errorReportingService.addMetadata('app', 'component', 'ui')
}

const ErrorBoundary = Bugsnag.getPlugin('react')?.createErrorBoundary(React) || React.Fragment

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ErrorBoundary>
      <BrowserRouter>
        <ThemeProvider>
          <QueryClientProvider client={queryClient}>
            <App />
            <ReactQueryDevtools initialIsOpen={false} />
          </QueryClientProvider>
        </ThemeProvider>
      </BrowserRouter>
    </ErrorBoundary>
  </StrictMode>
)
