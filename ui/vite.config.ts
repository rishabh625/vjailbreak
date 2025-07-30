import react from "@vitejs/plugin-react"
import path from "path"
import { defineConfig, loadEnv } from "vite"
import runtimeEnv from "vite-plugin-runtime-env"

export default defineConfig(({ mode }) => {
  // Load env variables based on the current mode (development, production, etc.)
  const env = loadEnv(mode, process.cwd())

  return {
    plugins: [
      runtimeEnv(),
      react({
        jsxImportSource: "@emotion/react",
        babel: {
          plugins: ["@emotion/babel-plugin"],
        },
      }),
    ],
    server: {
      port: 3000,
      // For dev mode: proxy api requests with the /dev-api prefix to the target server.
      proxy: {
        "/dev-api": {
          target: `${env.VITE_API_HOST}`,
          changeOrigin: true,
          secure: false,
          headers: {
            Authorization: `Bearer ${env.VITE_API_TOKEN}`,
          },
          rewrite: (path) => path.replace(/^\/dev-api/, ""),
        },
      },
    },
    base: "/",
    resolve: {
      alias: {
        "app-config": path.resolve(__dirname, "config.ts"),
        src: path.resolve(__dirname, "src"),
      },
    },
  }
})
