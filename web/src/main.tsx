import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import App from "./App";
import "./styles.css";

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: false, staleTime: 10_000 } },
});

const root = createRoot(document.getElementById("root")!);
const requestedPrototype = new URLSearchParams(window.location.search).get(
  "service-health",
);
const prototypeVariant = ["workbench", "matrix", "timeline"].includes(
  requestedPrototype ?? "",
)
  ? requestedPrototype
  : null;

async function render() {
  let content = <App />;
  if (import.meta.env.DEV && prototypeVariant) {
    const { ServiceHealthPrototype } = await import(
      "./dev/ServiceHealthPrototype"
    );
    content = <ServiceHealthPrototype variant={prototypeVariant} />;
  }

  root.render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>{content}</QueryClientProvider>
    </StrictMode>,
  );
}

render().catch(() => {
  root.render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <App />
      </QueryClientProvider>
    </StrictMode>,
  );
});
