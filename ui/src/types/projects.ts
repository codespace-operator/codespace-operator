export type UIProject = {
  kind: "Project";
  apiVersion: "codespace.codespace.dev/v1";
  metadata: {
    name: string;
    namespace: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    creationTimestamp?: string;
  };
  spec: {
    displayName?: string;
    description?: string;
    members?: ProjectMember[];
    namespaces?: string[];
    resourceQuotas?: {
      maxSessions?: number;
      maxReplicas?: number;
      maxCpuPerSession?: string;
      maxMemoryPerSession?: string;
    };
    defaultSessionProfile?: {
      ide?: "jupyterlab" | "vscode" | "rstudio" | "custom";
      image?: string;
      cmd?: string[];
    };
  };
  status?: {
    phase?: "Active" | "Inactive" | "Error";
    memberCount?: number;
    sessionCount?: number;
    reason?: string;
  };
};

export type ProjectMember = {
  subject: string; // user ID or group name
  role: "owner" | "admin" | "developer" | "viewer";
  addedAt?: string;
  addedBy?: string;
};

export type ProjectCreateRequest = {
  name: string;
  namespace?: string;
  displayName?: string;
  description?: string;
  members?: ProjectMember[];
  namespaces?: string[];
  resourceQuotas?: {
    maxSessions?: number;
    maxReplicas?: number;
    maxCpuPerSession?: string;
    maxMemoryPerSession?: string;
  };
  defaultSessionProfile?: {
    ide?: "jupyterlab" | "vscode" | "rstudio" | "custom";
    image?: string;
    cmd?: string[];
  };
};

export type ProjectUpdateRequest = Partial<ProjectCreateRequest>;

export type ProjectDeleteResponse = {
  status: string;
  name: string;
  namespace: string;
};

// Add to ui/src/api/client.ts
export const projectsApi = {
  async list(ns: string): Promise<UIProject[]> {
    const url =
      ns === "All"
        ? `/api/v1/server/projects?all=true`
        : `/api/v1/server/projects?namespace=${encodeURIComponent(ns)}`;
    const r = await apiFetch(url);
    const data = await r.json();

    if (data.items) {
      return data.items as UIProject[];
    }
    return normalizeList<UIProject>(data);
  },

  async get(ns: string, name: string): Promise<UIProject> {
    const r = await apiFetch(
      `/api/v1/server/projects/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`,
    );
    return normalizeObject<UIProject>(await r.json());
  },

  async create(body: ProjectCreateRequest): Promise<UIProject> {
    const r = await apiFetch(`/api/v1/server/projects`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    return normalizeObject<UIProject>(await r.json());
  },

  async update(
    ns: string,
    name: string,
    body: ProjectUpdateRequest,
  ): Promise<UIProject> {
    const r = await apiFetch(
      `/api/v1/server/projects/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`,
      {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      },
    );
    return normalizeObject<UIProject>(await r.json());
  },

  async remove(ns: string, name: string): Promise<ProjectDeleteResponse> {
    const r = await apiFetch(
      `/api/v1/server/projects/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`,
      {
        method: "DELETE",
      },
    );

    if (r.headers.get("content-length") === "0" || r.status === 204) {
      return { status: "deleted", name, namespace: ns };
    }
    return await r.json();
  },

  async addMember(
    ns: string,
    name: string,
    member: ProjectMember,
  ): Promise<UIProject> {
    const r = await apiFetch(
      `/api/v1/server/projects/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/members`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(member),
      },
    );
    return normalizeObject<UIProject>(await r.json());
  },

  async removeMember(
    ns: string,
    name: string,
    subject: string,
  ): Promise<UIProject> {
    const r = await apiFetch(
      `/api/v1/server/projects/${encodeURIComponent(ns)}/${encodeURIComponent(name)}/members/${encodeURIComponent(subject)}`,
      {
        method: "DELETE",
      },
    );
    return normalizeObject<UIProject>(await r.json());
  },
};
