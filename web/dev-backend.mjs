import { isIP } from "node:net";

export function validateDevBackend(value) {
  const backend = new URL(value);
  const hostname = backend.hostname.replace(/^\[|\]$/g, "");
  const ipVersion = isIP(hostname);
  const loopback =
    (ipVersion === 4 && hostname.startsWith("127.")) ||
    (ipVersion === 6 && hostname === "::1");
  if (
    backend.protocol !== "http:" ||
    !loopback ||
    backend.username ||
    backend.password ||
    (backend.pathname !== "/" && backend.pathname !== "") ||
    backend.search ||
    backend.hash
  ) {
    throw new Error(
      "LOG_LEOPARD_DEV_BACKEND must be an HTTP origin on a numeric loopback address",
    );
  }
  return backend.origin;
}
