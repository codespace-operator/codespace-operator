import React from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import "./styles.css";
import App from "./App";
import { ErrorBoundary } from "./ErrorBoundary";

document.documentElement.classList.add("pf-v6-theme-dark", "pf-v5-theme-dark");

const root = createRoot(document.getElementById("root")!);
root.render(
  <ErrorBoundary>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </ErrorBoundary>
);
