// Shared types that mirror the Session CRD (fields the UI needs)
export type Session = {
  metadata: { name: string; namespace: string };
  spec: {
    profile: {
      ide: "jupyterlab" | "vscode" | "rstudio" | "custom";
      image: string;
      cmd?: string[];
    };
    networking?: { host?: string };
    replicas?: number;
  };
  status?: { phase?: string; url?: string; reason?: string };
};

export type SessionEvent = { type: string; object: Session };
