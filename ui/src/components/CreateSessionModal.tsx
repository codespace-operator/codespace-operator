// src/components/CreateSessionModal.tsx
import React, { useState, useEffect } from "react";
import {
  Button,
  Modal,
  ModalVariant,
  Form,
  FormGroup,
  TextInput,
  FormSelect,
  FormSelectOption
} from "@patternfly/react-core";
import type { Session } from "../types";

type Props = {
  isOpen: boolean;
  /** The currently selected listing namespace from Header (can be "All") */
  namespace: string;
  onClose: () => void;
  onCreate: (body: Partial<Session>) => Promise<void>;
  /** RBAC-filtered namespaces where the user can CREATE sessions */
  writableNamespaces: string[];
};

export function CreateSessionModal({
  isOpen,
  namespace,
  onClose,
  onCreate,
  writableNamespaces = [], // <-- default to []
}: Props) {
  const [cNamespace, setCNamespace] = useState<string>("");
  const [cName, setCName] = useState("");
  const [cReplicas, setCReplicas] = useState<number>(1);
  const [cIDE, setCIDE] = useState<"jupyterlab" | "vscode">("jupyterlab");
  const [cImage, setCImage] = useState<string>("jupyter/minimal-notebook:latest");
  const [cCmd, setCCmd] = useState<string>("start-notebook.sh --NotebookApp.token=");
  const [cHost, setCHost] = useState<string>("");

  // When the modal opens or inputs change, choose a sensible default namespace:
  //  - If current listing namespace is writable (and not "All"), use it
  //  - Otherwise, use the first writable namespace (if any)
  useEffect(() => {
    const preferred =
      namespace && namespace !== "All" && writableNamespaces.includes(namespace)
        ? namespace
        : "";
    const next = preferred || writableNamespaces[0] || "";
    setCNamespace(next);
  }, [namespace, writableNamespaces, isOpen]);

  const handleIDEChange = (value: string) => {
    const ide = value as "jupyterlab" | "vscode";
    setCIDE(ide);
    if (ide === "vscode") {
      setCImage("codercom/code-server:latest");
      setCCmd("--bind-addr 0.0.0.0:8080 --auth none");
    } else {
      setCImage("jupyter/minimal-notebook:latest");
      setCCmd("start-notebook.sh --NotebookApp.token=");
    }
  };

  const submit = async () => {
    if (!cName || !cNamespace) return;
    const body: Partial<Session> = {
      metadata: { name: cName, namespace: cNamespace },
      spec: {
        replicas: cReplicas,
        profile: {
          ide: cIDE,
          image: cImage,
          cmd: cCmd.trim() ? cCmd.split(/\s+/) : undefined
        },
        networking: cHost ? { host: cHost } : undefined
      }
    } as any;

    await onCreate(body);

    // Reset form after successful creation
    setCName("");
    setCReplicas(1);
    if (cIDE === "vscode") {
      setCImage("codercom/code-server:latest");
      setCCmd("--bind-addr 0.0.0.0:8080 --auth none");
    } else {
      setCImage("jupyter/minimal-notebook:latest");
      setCCmd("start-notebook.sh --NotebookApp.token=");
    }
    setCHost("");
  };

  // Body content (keeps the "buttons in body" approach you already had)
  const renderContent = () => (
    <>
      <Form style={{ paddingBottom: "20px" }}>
        <FormGroup label="Namespace" isRequired fieldId="ns" style={{ marginBottom: "16px" }}>
          <FormSelect
            id="ns"
            value={cNamespace}
            onChange={(_, v) => setCNamespace(v)}
            isDisabled={writableNamespaces.length === 0}
          >
            {writableNamespaces.length === 0 ? (
              <FormSelectOption value="" label="No writable namespaces" isDisabled />
            ) : (
              writableNamespaces.map((ns) => (
                <FormSelectOption key={ns} value={ns} label={ns} />
              ))
            )}
          </FormSelect>
        </FormGroup>

        <FormGroup label="Name" isRequired fieldId="name" style={{ marginBottom: "16px" }}>
          <TextInput
            id="name"
            value={cName}
            onChange={(_, v) => setCName(v)}
            placeholder="Enter session name"
          />
        </FormGroup>

        <FormGroup label="Replicas" fieldId="replicas" style={{ marginBottom: "16px" }}>
          <TextInput
            id="replicas"
            type="number"
            value={String(cReplicas)}
            onChange={(_, v) => setCReplicas(Math.max(0, parseInt(v || "0")))}
            min={0}
          />
        </FormGroup>

        <FormGroup label="IDE" fieldId="ide" style={{ marginBottom: "16px" }}>
          <FormSelect id="ide" value={cIDE} onChange={(_, value) => handleIDEChange(value)}>
            <FormSelectOption value="jupyterlab" label="JupyterLab" />
            <FormSelectOption value="vscode" label="VS Code (code-server)" />
          </FormSelect>
        </FormGroup>

        <FormGroup label="Image" isRequired fieldId="image" style={{ marginBottom: "16px" }}>
          <TextInput
            id="image"
            value={cImage}
            onChange={(_, v) => setCImage(v)}
            placeholder="Container image"
          />
        </FormGroup>

        <FormGroup label="Command" fieldId="cmd" style={{ marginBottom: "16px" }}>
          <TextInput
            id="cmd"
            value={cCmd}
            onChange={(_, v) => setCCmd(v)}
            placeholder="Start command (optional)"
          />
        </FormGroup>

        <FormGroup label="Host" fieldId="host" style={{ marginBottom: "16px" }}>
          <TextInput
            id="host"
            value={cHost}
            onChange={(_, v) => setCHost(v)}
            placeholder="Custom hostname (optional)"
          />
        </FormGroup>
      </Form>

      {/* Action buttons rendered in body (your original approach) */}
      <div
        style={{
          display: "flex",
          justifyContent: "flex-end",
          gap: "12px",
          paddingTop: "16px",
          borderTop: "1px solid rgba(255,255,255,0.1)",
          marginTop: "auto"
        }}
      >
        <Button
          variant="primary"
          onClick={submit}
          isDisabled={!cName || !cNamespace || writableNamespaces.length === 0}
          style={{
            backgroundColor: "#0066cc",
            color: "#ffffff",
            border: "none",
            padding: "8px 24px",
            borderRadius: "3px",
            cursor: cName && cNamespace ? "pointer" : "not-allowed",
            opacity: cName && cNamespace ? 1 : 0.5
          }}
        >
          Create
        </Button>
        <Button
          variant="link"
          onClick={onClose}
          style={{
            color: "#6aaeff",
            background: "transparent",
            border: "none",
            padding: "8px 24px",
            cursor: "pointer"
          }}
        >
          Cancel
        </Button>
      </div>
    </>
  );

  return (
    <Modal
      variant={ModalVariant.medium}
      title="Create Session"
      isOpen={isOpen}
      onClose={onClose}
      showClose={false}
      hasNoBodyWrapper
      style={{
        "--pf-c-modal-box--MaxHeight": "90vh",
        "--pf-c-modal-box__body--MinHeight": "400px"
      } as any}
    >
      <div
        style={{
          padding: "24px",
          minHeight: "400px",
          display: "flex",
          flexDirection: "column"
        }}
      >
        {renderContent()}
      </div>
    </Modal>
  );
}
