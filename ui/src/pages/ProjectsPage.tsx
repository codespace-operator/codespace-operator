// ui/src/pages/ProjectsPage.tsx
import { useState, useMemo, useImperativeHandle, forwardRef } from "react";
import {
  PageSection,
  TextInput,
  Button,
  Card,
  CardBody,
  Title,
  Grid,
  GridItem,
} from "@patternfly/react-core";
import { PlusCircleIcon } from "@patternfly/react-icons";
import { ProjectsTable } from "../components/ProjectsTable";
import { CreateProjectModal } from "../components/CreateProjectModal";
import { ProjectDetailsModal } from "../components/ProjectDetailsModal";
import { useProjects, useFilteredProjects } from "../hooks/useProjects";
import { useIntrospection } from "../hooks/useIntrospection";
import type { UIProject, ProjectCreateRequest } from "../types/projects";

type Props = {
  namespace: string;
  onAlert: (
    message: string,
    variant: "success" | "danger" | "warning" | "info",
  ) => void;
};

export type ProjectsPageRef = {
  refresh: () => void;
};

export const ProjectsPage = forwardRef<ProjectsPageRef, Props>(
  ({ namespace, onAlert }, ref) => {
    const { data: ix, loading: ixLoading } = useIntrospection({
      discover: true,
    });

    const [query, setQuery] = useState("");
    const [isCreateOpen, setCreateOpen] = useState(false);
    const [selectedProject, setSelectedProject] = useState<UIProject | null>(
      null,
    );

    // Derive creatable namespaces from /introspect
    const creatableNamespaces = useMemo(() => {
      if (!ix) return [];
      const fromApi = ix.namespaces?.userCreatable;
      if (fromApi?.length) return [...fromApi].sort();

      const starCreate = !!ix.domains?.["*"]?.session?.create;
      if (starCreate) {
        const allowed = ix.namespaces?.userAllowed ?? [];
        return [...allowed].sort();
      }

      return Object.entries(ix.domains || {})
        .filter(([ns, perms]) => ns !== "*" && perms?.session?.create)
        .map(([ns]) => ns)
        .sort();
    }, [ix]);

    // Projects data
    const { rows, loading, refresh, create, remove, update, pendingTargets } =
      useProjects(namespace, (msg) => onAlert(msg, "danger"));
    const filtered = useFilteredProjects(rows, query);

    // Expose refresh function to parent component
    useImperativeHandle(
      ref,
      () => ({
        refresh,
      }),
      [refresh],
    );

    const openProject = (project: UIProject) => {
      setSelectedProject(project);
    };

    const doDelete = async (project: UIProject) => {
      if (
        !confirm(
          `Delete project ${project.metadata.name}? This will remove all project associations but not the sessions themselves.`,
        )
      )
        return;
      try {
        await remove(project.metadata.namespace, project.metadata.name);
        onAlert("Project deleted", "success");
        refresh();
      } catch (e: any) {
        onAlert(e?.message || "Delete failed", "danger");
      }
    };

    const handleCreate = async (body: ProjectCreateRequest) => {
      try {
        await create(body);
        onAlert(`Project ${body.name} created`, "success");
        setCreateOpen(false);
        refresh();
      } catch (e: any) {
        onAlert(e?.message || "Create failed", "danger");
      }
    };

    const handleUpdate = async (
      ns: string,
      name: string,
      body: ProjectCreateRequest,
    ) => {
      try {
        await update(ns, name, body);
        onAlert(`Project ${name} updated`, "success");
        setSelectedProject(null);
        refresh();
      } catch (e: any) {
        onAlert(e?.message || "Update failed", "danger");
      }
    };

    return (
      <>
        <PageSection className="projects-header">
          <div className="projects-header-content">
            <div>
              <Title headingLevel="h1" size="2xl">
                Projects
              </Title>
              <p className="pf-u-color-200 pf-u-mt-sm">
                Organize sessions and manage team access with project-based
                scoping
              </p>
            </div>
            <div className="projects-actions">
              <TextInput
                aria-label="Search projects"
                value={query}
                onChange={(_, v) => setQuery(v)}
                placeholder="Filter projects..."
                className="projects-search"
              />
              <Button
                icon={<PlusCircleIcon />}
                variant="primary"
                onClick={() => setCreateOpen(true)}
                isDisabled={creatableNamespaces.length === 0 || ixLoading}
                title={
                  creatableNamespaces.length === 0
                    ? "You lack 'create' permission in any namespace"
                    : undefined
                }
              >
                Create Project
              </Button>
            </div>
          </div>
        </PageSection>

        <PageSection isFilled className="projects-content">
          <Grid hasGutter>
            <GridItem span={12}>
              <Card className="projects-table-card">
                <CardBody>
                  <ProjectsTable
                    loading={loading}
                    rows={filtered}
                    pendingTargets={pendingTargets}
                    onDelete={doDelete}
                    onOpen={openProject}
                  />
                </CardBody>
              </Card>
            </GridItem>
          </Grid>

          <CreateProjectModal
            isOpen={isCreateOpen}
            namespace={namespace}
            writableNamespaces={creatableNamespaces}
            onClose={() => setCreateOpen(false)}
            onCreate={handleCreate}
          />

          {selectedProject && (
            <ProjectDetailsModal
              project={selectedProject}
              onClose={() => setSelectedProject(null)}
              onUpdate={handleUpdate}
              onAlert={onAlert}
            />
          )}
        </PageSection>
      </>
    );
  },
);
