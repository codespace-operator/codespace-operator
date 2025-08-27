import React from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import "./styles.css";
import App from "./App";
import { ErrorBoundary } from "./ErrorBoundary";
import { IntrospectionProvider } from "./context/IntrospectionContext";

document.documentElement.classList.add("pf-v6-theme-dark", "pf-v5-theme-dark");

const root = createRoot(document.getElementById("root")!);
root.render(
  <ErrorBoundary>
    <BrowserRouter>
      <IntrospectionProvider>
        <App />
      </IntrospectionProvider>
    </BrowserRouter>
  </ErrorBoundary>,
);
