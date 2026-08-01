import { fileURLToPath } from "node:url";

import webConfig from "./web/vite.config.ts";
import { validateDevBackend } from "./web/dev-backend.mjs";

const backend = validateDevBackend(
  process.env.LOG_LEOPARD_DEV_BACKEND ?? "http://127.0.0.1:8787",
);
const backendOrigin = backend;

export default {
  ...webConfig,
  root: fileURLToPath(new URL("./web", import.meta.url)),
  server: {
    proxy: {
      "/api": {
        target: backend,
        changeOrigin: true,
        configure(proxy) {
          proxy.on("proxyReq", (request) =>
            request.setHeader("Origin", backendOrigin),
          );
        },
      },
      "/openapi.json": { target: backend, changeOrigin: true },
    },
  },
};
