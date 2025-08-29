// ui/src/components/CreateProjectModal.tsx
import React, { useState, useEffect } from "react";
import {
  Button,
  Modal,
  ModalVariant,
  Form,
  FormGroup,
  TextInput,
  TextArea,
  FormSelect,
  FormSelectOption,
  Checkbox,
  NumberInput,
} from "@patternfly/react-core";
import type { ProjectCreateRequest } from "../types/projects";

type Props = {
  isOpen: boolean;
  namespace: string;
  onClose: () => void;
  onCreate: (body: ProjectCreateRequest) => Promise<void>;
  writableNamespaces: string[];
};

export function CreateProjectModal({
  isOpen,
  namespace,
  onClose,
  onCreate,
  writableNamespaces = [],
}: Props) {
  const [formData, setFormData] = useState<ProjectCreateRequest>({
    name: "",
    namespace: "",
    displayName: "",
    description: "",
    namespaces: [],
    resourceQuotas: {
      maxSessions: 10,
      maxReplicas: 3,
      maxCpuPerSession: "2",
      maxMemoryPerSession: "4Gi",
    },
    defaultSessionProfile: {
      ide: "jupyterlab",
      image: "jupyter/minimal-notebook:latest",
      cmd: ["start-notebook.sh", "--NotebookApp.token="],
    },
    members: [],
  });

  const [availableNamespaces, setAvailableNamespaces] = useState<string[]>([]);
  const [selectedNamespaces, setSelectedNamespaces] = useState<Set<string>>(
    new Set(),
  );

  // Initialize form when modal opens
  useEffect(() => {
    if (isOpen) {
      // Choose default namespace
      const preferred =
        namespace &&
        namespace !== "All" &&
        writableNamespaces.includes(namespace)
          ? namespace
          : writableNamespaces[0] || "";

      setFormData((prev) => ({
        ...prev,
        namespace: preferred,
      }));

      // Mock available namespaces - in real app, fetch from API
      setAvailableNamespaces([
        "default",
        "development",
        "staging",
        "production",
      ]);
      setSelectedNamespaces(new Set());
    }
  }, [isOpen, namespace, writableNamespaces]);

  const handleNamespaceToggle = (ns: string, checked: boolean) => {
    const newSelected = new Set(selectedNamespaces);
    if (checked) {
      newSelected.add(ns);
    } else {
      newSelected.delete(ns);
    }
    setSelectedNamespaces(newSelected);
    setFormData((prev) => ({
      ...prev,
      namespaces: Array.from(newSelected),
    }));
  };

  const handleIDEChange = (value: string) => {
    const ide = value as "jupyterlab" | "vscode" | "rstudio" | "custom";
    let image = "jupyter/minimal-notebook:latest";
    let cmd = ["start-notebook.sh", "--NotebookApp.token="];

    if (ide === "vscode") {
      image = "codercom/code-server:latest";
      cmd = ["--bind-addr", "0.0.0.0:8080", "--auth", "none"];
    } else if (ide === "rstudio") {
      image = "rocker/rstudio:latest";
      cmd = [];
    }

    setFormData((prev) => ({
      ...prev,
      defaultSessionProfile: {
        ...prev.defaultSessionProfile!,
        ide,
        image,
        cmd,
      },
    }));
  };

  const submit = async () => {
    if (!formData.name || !formData.namespace) return;

    await onCreate({
      ...formData,
      namespaces: formData.namespaces?.length ? formData.namespaces : undefined,
    });

    // Reset form
    setFormData({
      name: "",
      namespace: "",
      displayName: "",
      description: "",
      namespaces: [],
      resourceQuotas: {
        maxSessions: 10,
        maxReplicas: 3,
        maxCpuPerSession: "2",
        maxMemoryPerSession: "4Gi",
      },
      defaultSessionProfile: {
        ide: "jupyterlab",
        image: "jupyter/minimal-notebook:latest",
        cmd: ["start-notebook.sh", "--NotebookApp.token="],
      },
      members: [],
    });
    setSelectedNamespaces(new Set());
  };

  return (
    <Modal
      variant={ModalVariant.large}
      title="Create Project"
      isOpen={isOpen}
      onClose={onClose}
      showClose={false}
      hasNoBodyWrapper
    >
      <div
        style={{
          padding: "24px",
          minHeight: "500px",
          display: "flex",
          flexDirection: "column",
        }}
      >
        <Form style={{ paddingBottom: "20px" }}>
          {/* Basic Information */}
          <div style={{ marginBottom: "24px" }}>
            <h3
              style={{
                margin: "0 0 16px 0",
                fontSize: "1.1rem",
                fontWeight: 600,
              }}
            >
              Basic Information
            </h3>

            <FormGroup
              label="Project Namespace"
              isRequired
              fieldId="project-namespace"
            >
              <FormSelect
                id="project-namespace"
                value={formData.namespace}
                onChange={(_, v) =>
                  setFormData((prev) => ({ ...prev, namespace: v }))
                }
                isDisabled={writableNamespaces.length === 0}
              >
                {writableNamespaces.length === 0 ? (
                  <FormSelectOption
                    value=""
                    label="No creatable namespaces"
                    isDisabled
                  />
                ) : (
                  writableNamespaces.map((ns) => (
                    <FormSelectOption key={ns} value={ns} label={ns} />
                  ))
                )}
              </FormSelect>
            </FormGroup>

            <FormGroup label="Project Name" isRequired fieldId="project-name">
              <TextInput
                id="project-name"
                value={formData.name}
                onChange={(_, v) =>
                  setFormData((prev) => ({ ...prev, name: v }))
                }
                placeholder="my-project"
              />
            </FormGroup>

            <FormGroup label="Display Name" fieldId="project-display-name">
              <TextInput
                id="project-display-name"
                value={formData.displayName || ""}
                onChange={(_, v) =>
                  setFormData((prev) => ({ ...prev, displayName: v }))
                }
                placeholder="My Project"
              />
            </FormGroup>

            <FormGroup label="Description" fieldId="project-description">
              <TextArea
                id="project-description"
                value={formData.description || ""}
                onChange={(_, v) =>
                  setFormData((prev) => ({ ...prev, description: v }))
                }
                placeholder="Project description..."
                rows={3}
              />
            </FormGroup>
          </div>

          {/* Scoped Namespaces */}
          <div style={{ marginBottom: "24px" }}>
            <h3
              style={{
                margin: "0 0 16px 0",
                fontSize: "1.1rem",
                fontWeight: 600,
              }}
            >
              Namespace Scope
            </h3>
            <p
              style={{
                margin: "0 0 16px 0",
                color: "var(--pf-global--Color--200)",
                fontSize: "0.875rem",
              }}
            >
              Leave empty to allow sessions in any namespace, or select specific
              namespaces to restrict scope.
            </p>

            <div
              style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fit, minmax(200px, 1fr))",
                gap: "8px",
              }}
            >
              {availableNamespaces.map((ns) => (
                <Checkbox
                  key={ns}
                  id={`ns-${ns}`}
                  label={ns}
                  isChecked={selectedNamespaces.has(ns)}
                  onChange={(_, checked) => handleNamespaceToggle(ns, checked)}
                />
              ))}
            </div>
          </div>

          {/* Resource Quotas */}
          <div style={{ marginBottom: "24px" }}>
            <h3
              style={{
                margin: "0 0 16px 0",
                fontSize: "1.1rem",
                fontWeight: 600,
              }}
            >
              Resource Limits
            </h3>

            <div
              style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fit, minmax(200px, 1fr))",
                gap: "16px",
              }}
            >
              <FormGroup label="Max Sessions" fieldId="max-sessions">
                <NumberInput
                  value={formData.resourceQuotas?.maxSessions || 10}
                  onMinus={() =>
                    setFormData((prev) => ({
                      ...prev,
                      resourceQuotas: {
                        ...prev.resourceQuotas!,
                        maxSessions: Math.max(
                          1,
                          (prev.resourceQuotas?.maxSessions || 10) - 1,
                        ),
                      },
                    }))
                  }
                  onChange={(e) => {
                    const val =
                      parseInt((e.target as HTMLInputElement).value) || 1;
                    setFormData((prev) => ({
                      ...prev,
                      resourceQuotas: {
                        ...prev.resourceQuotas!,
                        maxSessions: val,
                      },
                    }));
                  }}
                  onPlus={() =>
                    setFormData((prev) => ({
                      ...prev,
                      resourceQuotas: {
                        ...prev.resourceQuotas!,
                        maxSessions:
                          (prev.resourceQuotas?.maxSessions || 10) + 1,
                      },
                    }))
                  }
                  min={1}
                />
              </FormGroup>

              <FormGroup
                label="Max Replicas per Session"
                fieldId="max-replicas"
              >
                <NumberInput
                  value={formData.resourceQuotas?.maxReplicas || 3}
                  onMinus={() =>
                    setFormData((prev) => ({
                      ...prev,
                      resourceQuotas: {
                        ...prev.resourceQuotas!,
                        maxReplicas: Math.max(
                          1,
                          (prev.resourceQuotas?.maxReplicas || 3) - 1,
                        ),
                      },
                    }))
                  }
                  onChange={(e) => {
                    const val =
                      parseInt((e.target as HTMLInputElement).value) || 1;
                    setFormData((prev) => ({
                      ...prev,
                      resourceQuotas: {
                        ...prev.resourceQuotas!,
                        maxReplicas: val,
                      },
                    }));
                  }}
                  onPlus={() =>
                    setFormData((prev) => ({
                      ...prev,
                      resourceQuotas: {
                        ...prev.resourceQuotas!,
                        maxReplicas:
                          (prev.resourceQuotas?.maxReplicas || 3) + 1,
                      },
                    }))
                  }
                  min={1}
                />
              </FormGroup>

              <FormGroup label="Max CPU per Session" fieldId="max-cpu">
                <TextInput
                  value={formData.resourceQuotas?.maxCpuPerSession || "2"}
                  onChange={(_, v) =>
                    setFormData((prev) => ({
                      ...prev,
                      resourceQuotas: {
                        ...prev.resourceQuotas!,
                        maxCpuPerSession: v,
                      },
                    }))
                  }
                  placeholder="2"
                />
              </FormGroup>

              <FormGroup label="Max Memory per Session" fieldId="max-memory">
                <TextInput
                  value={formData.resourceQuotas?.maxMemoryPerSession || "4Gi"}
                  onChange={(_, v) =>
                    setFormData((prev) => ({
                      ...prev,
                      resourceQuotas: {
                        ...prev.resourceQuotas!,
                        maxMemoryPerSession: v,
                      },
                    }))
                  }
                  placeholder="4Gi"
                />
              </FormGroup>
            </div>
          </div>

          {/* Default Session Profile */}
          <div style={{ marginBottom: "24px" }}>
            <h3
              style={{
                margin: "0 0 16px 0",
                fontSize: "1.1rem",
                fontWeight: 600,
              }}
            >
              Default Session Profile
            </h3>
            <p
              style={{
                margin: "0 0 16px 0",
                color: "var(--pf-global--Color--200)",
                fontSize: "0.875rem",
              }}
            >
              Set default values for new sessions created in this project.
            </p>

            <div
              style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fit, minmax(200px, 1fr))",
                gap: "16px",
              }}
            >
              <FormGroup label="Default IDE" fieldId="default-ide">
                <FormSelect
                  id="default-ide"
                  value={formData.defaultSessionProfile?.ide || "jupyterlab"}
                  onChange={(_, value) => handleIDEChange(value)}
                >
                  <FormSelectOption value="jupyterlab" label="JupyterLab" />
                  <FormSelectOption
                    value="vscode"
                    label="VS Code (code-server)"
                  />
                  <FormSelectOption value="rstudio" label="RStudio" />
                  <FormSelectOption value="custom" label="Custom" />
                </FormSelect>
              </FormGroup>

              <FormGroup label="Default Image" fieldId="default-image">
                <TextInput
                  id="default-image"
                  value={formData.defaultSessionProfile?.image || ""}
                  onChange={(_, v) =>
                    setFormData((prev) => ({
                      ...prev,
                      defaultSessionProfile: {
                        ...prev.defaultSessionProfile!,
                        image: v,
                      },
                    }))
                  }
                  placeholder="jupyter/minimal-notebook:latest"
                />
              </FormGroup>
            </div>
          </div>
        </Form>

        <div
          style={{
            display: "flex",
            justifyContent: "flex-end",
            gap: "12px",
            paddingTop: "16px",
            borderTop: "1px solid rgba(255,255,255,0.1)",
            marginTop: "auto",
          }}
        >
          <Button
            type="button"
            onClick={submit}
            variant="primary"
            isDisabled={
              !formData.name ||
              !formData.namespace ||
              writableNamespaces.length === 0
            }
          >
            Create Project
          </Button>
          <Button variant="link" onClick={onClose}>
            Cancel
          </Button>
        </div>
      </div>
    </Modal>
  );
}
