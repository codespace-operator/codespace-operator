import React, { useState } from "react";
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
  namespace: string; 
  onClose: () => void; 
  onCreate: (body: Partial<Session>) => Promise<void>; 
};

export function CreateSessionModal({ isOpen, namespace, onClose, onCreate }: Props) {
  const [cName, setCName] = useState("");
  const [cReplicas, setCReplicas] = useState<number>(1);
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
    // Reset form after successful creation
    setCName("");
    setCReplicas(1);
    setCImage("jupyter/minimal-notebook:latest");
    setCCmd("start-notebook.sh --NotebookApp.token=");
    setCHost("");
  };

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

  return (
    <Modal
      variant={ModalVariant.medium}
      title="Create Session"
      isOpen={isOpen}
      onClose={onClose}
      actions={[
        <Button 
          key="create" 
          variant="primary" 
          onClick={submit} 
          isDisabled={!cName}
        >
          Create
        </Button>,
        <Button key="cancel" variant="link" onClick={onClose}>
          Cancel
        </Button>,
      ]}
    >
      <Form>
        <FormGroup label="Name" isRequired fieldId="name">
          <TextInput 
            id="name" 
            value={cName} 
            onChange={(_, v) => setCName(v)} 
            placeholder="Enter session name"
          />
        </FormGroup>
        
        <FormGroup label="Replicas" fieldId="replicas">
          <TextInput
            id="replicas"
            type="number"
            value={String(cReplicas)}
            onChange={(_, v) => setCReplicas(Math.max(0, parseInt(v || "0")))}
            min={0}
          />
        </FormGroup>
        
        <FormGroup label="IDE" fieldId="ide">
          <FormSelect
            id="ide"
            value={cIDE}
            onChange={(_, value) => handleIDEChange(value)}
          >
            <FormSelectOption value="jupyterlab" label="JupyterLab" />
            <FormSelectOption value="vscode" label="VS Code (code-server)" />
          </FormSelect>
        </FormGroup>
        
        <FormGroup label="Image" isRequired fieldId="image">
          <TextInput 
            id="image" 
            value={cImage} 
            onChange={(_, v) => setCImage(v)} 
            placeholder="Container image"
          />
        </FormGroup>
        
        <FormGroup label="Command" fieldId="cmd">
          <TextInput 
            id="cmd" 
            value={cCmd} 
            onChange={(_, v) => setCCmd(v)} 
            placeholder="Start command (optional)"
          />
        </FormGroup>
        
        <FormGroup label="Host" fieldId="host">
          <TextInput 
            id="host" 
            value={cHost} 
            onChange={(_, v) => setCHost(v)} 
            placeholder="Custom hostname (optional)"
          />
        </FormGroup>
      </Form>
    </Modal>
  );
}