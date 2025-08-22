import React from "react";
import { createRoot } from "react-dom/client";
import "./styles.css";
import App from "./App";
createRoot(document.getElementById("root")!).render(<App />);

import { ErrorBoundary } from "./ErrorBoundary";
createRoot(document.getElementById("root")!).render(
  <ErrorBoundary><App /></ErrorBoundary>
);