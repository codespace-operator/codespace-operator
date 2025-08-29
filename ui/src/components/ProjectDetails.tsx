// ui/src/components/ProjectDetailsModal.tsx
import React, { useState, useEffect } from "react";
import {
  Modal,
  ModalVariant,
  Tabs,
  Tab,
  TabTitleText,
  Card,
  CardBody,
  Button,
  Form,
  FormGroup,
  TextInput,
  TextArea,
  FormSelect,
  FormSelectOption,
  Table,
  Thead,
  Tbody,
  Tr,
  Th,
  Td,
  Label,
  Badge,
  ActionList,
  ActionListItem,
  DescriptionList,
  DescriptionListGroup,
  DescriptionListTerm,
  DescriptionListDescription,
  Alert,
} from "@patternfly/react-core";
import {
  PencilAltIcon,
  TrashIcon,
  PlusIcon,
  UserIcon,
  CogIcon,
} from "@patternfly/react-icons";
import type {
  UIProject,
  ProjectMember,
  ProjectCreateRequest,
} from "../types/projects";

type Props = {
  project: UIProject;
  onClose: () => void;
  onUpdate: (
    ns: string,
    name: string,
    body: ProjectCreateRequest,
  ) => Promise<void>;
  onAlert: (
    message: string,
    variant: "success" | "danger" | "warning" | "info",
  ) => void;
};

export function ProjectDetailsModal({
  project,
  onClose,
  onUpdate,
  onAlert,
}: Props) {
  const [activeTab, setActiveTab] = useState<string | number>(0);
  const [isEditing, setIsEditing] = useState(false);
  const [formData, setFormData] = useState<ProjectCreateRequest>({
    name: project.metadata.name,
    namespace: project.metadata.namespace,
    displayName: project.spec.displayName || "",
    description: project.spec.description || "",
    namespaces: project.spec.namespaces || [],
    resourceQuotas: project.spec.resourceQuotas || {
      maxSessions: 10,
      maxReplicas: 3,
      maxCpuPerSession: "2",
      maxMemoryPerSession: "4Gi",
    },
    defaultSessionProfile: project.spec.defaultSessionProfile || {
      ide: "jupyterlab",
      image: "jupyter/minimal-notebook:latest",
      cmd: ["start-notebook.sh", "--NotebookApp.token="],
    },
    members: project.spec.members || [],
  });

  const [newMemberSubject, setNewMemberSubject] = useState("");
  const [newMemberRole, setNewMemberRole] = useState<
    "owner" | "admin" | "developer" | "viewer"
  >("developer");

  useEffect(() => {
    // Reset form data when project changes
    setFormData({
      name: project.metadata.name,
      namespace: project.metadata.namespace,
      displayName: project.spec.displayName || "",
      description: project.spec.description || "",
      namespaces: project.spec.namespaces || [],
      resourceQuotas: project.spec.resourceQuotas || {
        maxSessions: 10,
        maxReplicas: 3,
        maxCpuPerSession: "2",
        maxMemoryPerSession: "4Gi",
      },
      defaultSessionProfile: project.spec.defaultSessionProfile || {
        ide: "jupyterlab",
        image: "jupyter/minimal-notebook:latest",
        cmd: ["start-notebook.sh", "--NotebookApp.token="],
      },
      members: project.spec.members || [],
    });
    setIsEditing(false);
  }, [project]);

  const handleSave = async () => {
    try {
      await onUpdate(
        project.metadata.namespace,
        project.metadata.name,
        formData,
      );
      setIsEditing(false);
    } catch (error) {
      // Error handling is done by parent component
    }
  };

  const handleCancel = () => {
    // Reset form data
    setFormData({
      name: project.metadata.name,
      namespace: project.metadata.namespace,
      displayName: project.spec.displayName || "",
      description: project.spec.description || "",
      namespaces: project.spec.namespaces || [],
      resourceQuotas: project.spec.resourceQuotas || {
        maxSessions: 10,
        maxReplicas: 3,
        maxCpuPerSession: "2",
        maxMemoryPerSession: "4Gi",
      },
      defaultSessionProfile: project.spec.defaultSessionProfile || {
        ide: "jupyterlab",
        image: "jupyter/minimal-notebook:latest",
        cmd: ["start-notebook.sh", "--NotebookApp.token="],
      },
      members: project.spec.members || [],
    });
    setIsEditing(false);
  };

  const addMember = () => {
    if (!newMemberSubject.trim()) return;

    const newMember: ProjectMember = {
      subject: newMemberSubject.trim(),
      role: newMemberRole,
      addedAt: new Date().toISOString(),
      addedBy: "current-user", // TODO: Get from auth context
    };

    setFormData((prev) => ({
      ...prev,
      members: [...(prev.members || []), newMember],
    }));

    setNewMemberSubject("");
    setNewMemberRole("developer");
  };

  const removeMember = (index: number) => {
    setFormData((prev) => ({
      ...prev,
      members: prev.members?.filter((_, i) => i !== index) || [],
    }));
  };

  const getRoleColor = (role: string) => {
    switch (role) {
      case "owner":
        return "purple";
      case "admin":
        return "red";
      case "developer":
        return "blue";
      case "viewer":
        return "green";
      default:
        return "grey";
    }
  };

  const formatDate = (dateStr?: string) => {
    if (!dateStr) return "—";
    try {
      return new Date(dateStr).toLocaleDateString();
    } catch {
      return "—";
    }
  };

  const overviewTab = (
    <Card>
      <CardBody>
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "flex-start",
            marginBottom: "1.5rem",
          }}
        >
          <div>
            <h2 style={{ margin: 0, fontSize: "1.5rem", fontWeight: 600 }}>
              {project.spec.displayName || project.metadata.name}
            </h2>
            <div
              style={{
                color: "var(--pf-global--Color--200)",
                marginTop: "0.5rem",
              }}
            >
              {project.metadata.namespace} / {project.metadata.name}
            </div>
          </div>
          <div style={{ display: "flex", gap: "0.5rem" }}>
            {!isEditing ? (
              <Button
                variant="secondary"
                icon={<PencilAltIcon />}
                onClick={() => setIsEditing(true)}
                size="sm"
              >
                Edit
              </Button>
            ) : (
              <>
                <Button variant="primary" onClick={handleSave} size="sm">
                  Save Changes
                </Button>
                <Button variant="link" onClick={handleCancel} size="sm">
                  Cancel
                </Button>
              </>
            )}
          </div>
        </div>

        {isEditing ? (
          <Form>
            <FormGroup label="Display Name" fieldId="edit-display-name">
              <TextInput
                id="edit-display-name"
                value={formData.displayName}
                onChange={(_, v) =>
                  setFormData((prev) => ({ ...prev, displayName: v }))
                }
                placeholder="My Project"
              />
            </FormGroup>

            <FormGroup label="Description" fieldId="edit-description">
              <TextArea
                id="edit-description"
                value={formData.description}
                onChange={(_, v) =>
                  setFormData((prev) => ({ ...prev, description: v }))
                }
                placeholder="Project description..."
                rows={3}
              />
            </FormGroup>

            <FormGroup label="Max Sessions" fieldId="edit-max-sessions">
              <TextInput
                id="edit-max-sessions"
                type="number"
                value={String(formData.resourceQuotas?.maxSessions || 10)}
                onChange={(_, v) => {
                  const val = parseInt(v) || 10;
                  setFormData((prev) => ({
                    ...prev,
                    resourceQuotas: {
                      ...prev.resourceQuotas!,
                      maxSessions: val,
                    },
                  }));
                }}
                min={1}
              />
            </FormGroup>

            <FormGroup label="Max CPU per Session" fieldId="edit-max-cpu">
              <TextInput
                id="edit-max-cpu"
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

            <FormGroup label="Max Memory per Session" fieldId="edit-max-memory">
              <TextInput
                id="edit-max-memory"
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
          </Form>
        ) : (
          <DescriptionList isCompact>
            <DescriptionListGroup>
              <DescriptionListTerm>Description</DescriptionListTerm>
              <DescriptionListDescription>
                {project.spec.description || (
                  <em style={{ color: "var(--pf-global--Color--200)" }}>
                    No description
                  </em>
                )}
              </DescriptionListDescription>
            </DescriptionListGroup>

            <DescriptionListGroup>
              <DescriptionListTerm>Created</DescriptionListTerm>
              <DescriptionListDescription>
                {formatDate(project.metadata.creationTimestamp)}
              </DescriptionListDescription>
            </DescriptionListGroup>

            <DescriptionListGroup>
              <DescriptionListTerm>Status</DescriptionListTerm>
              <DescriptionListDescription>
                <Label
                  color={
                    project.status?.phase === "Active" ? "green" : "orange"
                  }
                >
                  {project.status?.phase || "Unknown"}
                </Label>
              </DescriptionListDescription>
            </DescriptionListGroup>

            <DescriptionListGroup>
              <DescriptionListTerm>Members</DescriptionListTerm>
              <DescriptionListDescription>
                <Badge isRead>
                  {project.status?.memberCount ||
                    project.spec.members?.length ||
                    0}
                </Badge>
              </DescriptionListDescription>
            </DescriptionListGroup>

            <DescriptionListGroup>
              <DescriptionListTerm>Active Sessions</DescriptionListTerm>
              <DescriptionListDescription>
                <Badge isRead>{project.status?.sessionCount || 0}</Badge>
                {project.spec.resourceQuotas?.maxSessions && (
                  <span
                    style={{
                      color: "var(--pf-global--Color--200)",
                      marginLeft: "0.5rem",
                    }}
                  >
                    / {project.spec.resourceQuotas.maxSessions} max
                  </span>
                )}
              </DescriptionListDescription>
            </DescriptionListGroup>

            <DescriptionListGroup>
              <DescriptionListTerm>Resource Limits</DescriptionListTerm>
              <DescriptionListDescription>
                <div
                  style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap" }}
                >
                  <Label isCompact color="blue">
                    CPU: {project.spec.resourceQuotas?.maxCpuPerSession || "2"}
                  </Label>
                  <Label isCompact color="blue">
                    Memory:{" "}
                    {project.spec.resourceQuotas?.maxMemoryPerSession || "4Gi"}
                  </Label>
                  <Label isCompact color="blue">
                    Replicas: {project.spec.resourceQuotas?.maxReplicas || 3}
                  </Label>
                </div>
              </DescriptionListDescription>
            </DescriptionListGroup>

            <DescriptionListGroup>
              <DescriptionListTerm>Scoped Namespaces</DescriptionListTerm>
              <DescriptionListDescription>
                {project.spec.namespaces &&
                project.spec.namespaces.length > 0 ? (
                  <div
                    style={{
                      display: "flex",
                      gap: "0.25rem",
                      flexWrap: "wrap",
                    }}
                  >
                    {project.spec.namespaces.map((ns) => (
                      <Label key={ns} isCompact color="purple">
                        {ns}
                      </Label>
                    ))}
                  </div>
                ) : (
                  <em style={{ color: "var(--pf-global--Color--200)" }}>
                    All namespaces
                  </em>
                )}
              </DescriptionListDescription>
            </DescriptionListGroup>
          </DescriptionList>
        )}
      </CardBody>
    </Card>
  );

  const membersTab = (
    <Card>
      <CardBody>
        <div style={{ marginBottom: "1.5rem" }}>
          <h3
            style={{
              margin: "0 0 1rem 0",
              fontSize: "1.25rem",
              fontWeight: 600,
            }}
          >
            Project Members
          </h3>

          {isEditing && (
            <Alert
              variant="info"
              isInline
              title="Member Management"
              style={{ marginBottom: "1rem" }}
            >
              Add or remove project members. Changes will be applied when you
              save the project.
            </Alert>
          )}

          {isEditing && (
            <div
              style={{
                display: "grid",
                gridTemplateColumns: "1fr auto auto",
                gap: "0.5rem",
                alignItems: "end",
                marginBottom: "1rem",
                padding: "1rem",
                backgroundColor: "var(--pf-global--BackgroundColor--200)",
                borderRadius: "3px",
              }}
            >
              <FormGroup label="User/Subject" fieldId="new-member-subject">
                <TextInput
                  id="new-member-subject"
                  value={newMemberSubject}
                  onChange={(_, v) => setNewMemberSubject(v)}
                  placeholder="user@company.com or username"
                />
              </FormGroup>

              <FormGroup label="Role" fieldId="new-member-role">
                <FormSelect
                  id="new-member-role"
                  value={newMemberRole}
                  onChange={(_, v) => setNewMemberRole(v as any)}
                >
                  <FormSelectOption value="owner" label="Owner" />
                  <FormSelectOption value="admin" label="Admin" />
                  <FormSelectOption value="developer" label="Developer" />
                  <FormSelectOption value="viewer" label="Viewer" />
                </FormSelect>
              </FormGroup>

              <Button
                variant="primary"
                icon={<PlusIcon />}
                onClick={addMember}
                isDisabled={!newMemberSubject.trim()}
              >
                Add Member
              </Button>
            </div>
          )}
        </div>

        <Table aria-label="Project members" variant="compact">
          <Thead>
            <Tr>
              <Th>User</Th>
              <Th>Role</Th>
              <Th>Added</Th>
              <Th>Added By</Th>
              {isEditing && <Th>Actions</Th>}
            </Tr>
          </Thead>
          <Tbody>
            {formData.members && formData.members.length > 0 ? (
              formData.members.map((member, index) => (
                <Tr key={index}>
                  <Td dataLabel="User">
                    <div
                      style={{
                        display: "flex",
                        alignItems: "center",
                        gap: "0.5rem",
                      }}
                    >
                      <UserIcon size="sm" />
                      {member.subject}
                    </div>
                  </Td>
                  <Td dataLabel="Role">
                    <Label color={getRoleColor(member.role)}>
                      {member.role}
                    </Label>
                  </Td>
                  <Td dataLabel="Added">{formatDate(member.addedAt)}</Td>
                  <Td dataLabel="Added By">{member.addedBy || "—"}</Td>
                  {isEditing && (
                    <Td dataLabel="Actions">
                      <Button
                        variant="plain"
                        icon={<TrashIcon />}
                        onClick={() => removeMember(index)}
                        aria-label="Remove member"
                        size="sm"
                      />
                    </Td>
                  )}
                </Tr>
              ))
            ) : (
              <Tr>
                <Td
                  colSpan={isEditing ? 5 : 4}
                  style={{
                    textAlign: "center",
                    padding: "2rem",
                    color: "var(--pf-global--Color--200)",
                  }}
                >
                  No members configured
                </Td>
              </Tr>
            )}
          </Tbody>
        </Table>
      </CardBody>
    </Card>
  );

  const settingsTab = (
    <Card>
      <CardBody>
        <h3
          style={{ margin: "0 0 1rem 0", fontSize: "1.25rem", fontWeight: 600 }}
        >
          Default Session Profile
        </h3>
        <p
          style={{
            margin: "0 0 1.5rem 0",
            color: "var(--pf-global--Color--200)",
          }}
        >
          These settings will be used as defaults when creating new sessions in
          this project.
        </p>

        {isEditing ? (
          <Form>
            <FormGroup label="Default IDE" fieldId="default-ide-edit">
              <FormSelect
                id="default-ide-edit"
                value={formData.defaultSessionProfile?.ide || "jupyterlab"}
                onChange={(_, value) => {
                  const ide = value as
                    | "jupyterlab"
                    | "vscode"
                    | "rstudio"
                    | "custom";
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
                }}
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

            <FormGroup label="Default Image" fieldId="default-image-edit">
              <TextInput
                id="default-image-edit"
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
          </Form>
        ) : (
          <DescriptionList isCompact>
            <DescriptionListGroup>
              <DescriptionListTerm>IDE</DescriptionListTerm>
              <DescriptionListDescription>
                <Label color="blue">
                  {project.spec.defaultSessionProfile?.ide || "jupyterlab"}
                </Label>
              </DescriptionListDescription>
            </DescriptionListGroup>

            <DescriptionListGroup>
              <DescriptionListTerm>Container Image</DescriptionListTerm>
              <DescriptionListDescription>
                <code style={{ fontSize: "0.875rem" }}>
                  {project.spec.defaultSessionProfile?.image ||
                    "jupyter/minimal-notebook:latest"}
                </code>
              </DescriptionListDescription>
            </DescriptionListGroup>

            <DescriptionListGroup>
              <DescriptionListTerm>Command</DescriptionListTerm>
              <DescriptionListDescription>
                {project.spec.defaultSessionProfile?.cmd &&
                project.spec.defaultSessionProfile.cmd.length > 0 ? (
                  <code style={{ fontSize: "0.875rem" }}>
                    {project.spec.defaultSessionProfile.cmd.join(" ")}
                  </code>
                ) : (
                  <em style={{ color: "var(--pf-global--Color--200)" }}>
                    Default entrypoint
                  </em>
                )}
              </DescriptionListDescription>
            </DescriptionListGroup>
          </DescriptionList>
        )}
      </CardBody>
    </Card>
  );

  return (
    <Modal
      variant={ModalVariant.large}
      title={`Project: ${project.metadata.name}`}
      isOpen={true}
      onClose={onClose}
      showClose={true}
      hasNoBodyWrapper
    >
      <div style={{ padding: "1.5rem", minHeight: "600px" }}>
        <Tabs
          activeKey={activeTab}
          onSelect={(_, tabIndex) => setActiveTab(tabIndex)}
          style={{ marginBottom: "1.5rem" }}
        >
          <Tab eventKey={0} title={<TabTitleText>Overview</TabTitleText>}>
            {overviewTab}
          </Tab>
          <Tab eventKey={1} title={<TabTitleText>Members</TabTitleText>}>
            {membersTab}
          </Tab>
          <Tab eventKey={2} title={<TabTitleText>Settings</TabTitleText>}>
            {settingsTab}
          </Tab>
        </Tabs>
      </div>
    </Modal>
  );
}
