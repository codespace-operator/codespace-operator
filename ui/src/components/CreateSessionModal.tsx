import React, { useState } from "react";
import { Button, Modal, Form, FormGroup, TextInput, Select, SelectOption, Checkbox } from "@patternfly/react-core";
import type { Session } from "../types";

type Props = {
  isOpen: boolean;
  namespace: string;
  onClose: () => void;
  onCreate: (body: Partial<Session>) => Promise<void>;
};

export function CreateSessionModal({ isOpen, namespace, onClose, onCreate }: Props) {
  const [cName, setCName] = useState("");
  const [cReplicas, setCReplicas] = useState<number>(1);
  const [cIDEOpen, setCIDEOpen] = useState<boolean>(false);
  const [cIDE, setCIDE] = useState<"jupyterlab" | "vscode">("jupyterlab");
  const [cImage, setCImage] = useState<string>("jupyter/minimal-notebook:latest");
  const [cCmd, setCCmd] = useState<string>("start-notebook.sh --NotebookApp.token=");
  const [cHost, setCHost] = useState<string>("");

  const submit = async () => {
    if (!cName) return;
    const body: Partial<Session> = {
      metadata: { name: cName, namespace },
      spec: {
        replicas: cReplicas,
        profile: { ide: cIDE, image: cImage, cmd: cCmd.trim() ? cCmd.split(/\s+/) : undefined },
        networking: cHost ? { host: cHost } : undefined,
      },
    } as any;
    await onCreate(body);
    setCName("");
  };

  return (
    <Modal
      title="Create Session"
      isOpen={isOpen}
      onClose={onClose}
      actions={[
        <Button key="create" variant="primary" onClick={submit} isDisabled={!cName}>
          Create
        </Button>,
        <Button key="cancel" variant="link" onClick={onClose}>
          Cancel
        </Button>,
      ]}
    >
      <Form>
        <FormGroup label="Name" isRequired fieldId="name">
          <TextInput id="name" value={cName} onChange={(_, v) => setCName(v)} />
        </FormGroup>
        <FormGroup label="Replicas" fieldId="replicas">
          <TextInput
            id="replicas"
            type="number"
            value={String(cReplicas)}
            onChange={(_, v) => setCReplicas(Math.max(0, parseInt(v || "0")))}
          />
        </FormGroup>
        <FormGroup label="IDE" fieldId="ide">
          <select
            id="ide"
            value={cIDE}
            onChange={(e) => {
              const val = e.target.value as "jupyterlab" | "vscode";
              setCIDE(val);
              if (val === "vscode") {
                setCImage("codercom/code-server:latest");
                setCCmd("--bind-addr 0.0.0.0:8080 --auth none");
              } else {
                setCImage("jupyter/minimal-notebook:latest");
                setCCmd("start-notebook.sh --NotebookApp.token=");
              }
            }}
            style={{ width: "100%", padding: 8 }}
          >
            <option value="jupyterlab">JupyterLab</option>
            <option value="vscode">VS Code (code-server)</option>
          </select>
        </FormGroup>
        <FormGroup label="Image" isRequired fieldId="image">
          <TextInput id="image" value={cImage} onChange={(_, v) => setCImage(v)} />
        </FormGroup>
        <FormGroup label="Command" fieldId="cmd">
          <TextInput id="cmd" value={cCmd} onChange={(_, v) => setCCmd(v)} />
        </FormGroup>
        <FormGroup label="Public Host (optional)" fieldId="host">
          <TextInput id="host" placeholder="lab.example.dev" value={cHost} onChange={(_, v) => setCHost(v)} />
        </FormGroup>
      </Form>
    </Modal>
  );
}
