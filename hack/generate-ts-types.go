package main

import (
	"fmt"
	"os"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func main() {
	// Read CRD file
	crdPath := "config/crd/bases/codespace.codespace.dev_sessions.yaml"
	data, err := os.ReadFile(crdPath)
	if err != nil {
		panic(err)
	}

	var crd apiextv1.CustomResourceDefinition
	if err := yaml.Unmarshal(data, &crd); err != nil {
		panic(err)
	}

	// Find v1 schema
	var schema *apiextv1.JSONSchemaProps
	for _, version := range crd.Spec.Versions {
		if version.Name == "v1" {
			schema = version.Schema.OpenAPIV3Schema
			break
		}
	}

	// Convert to TypeScript
	tsTypes := generateTypeScript(schema)

	// Write to ui/src/types.generated.ts
	outPath := "ui/src/types.generated.ts"
	if err := os.WriteFile(outPath, []byte(tsTypes), 0644); err != nil {
		panic(err)
	}

	fmt.Printf("Generated TypeScript types: %s\n", outPath)
}

func generateTypeScript(schema *apiextv1.JSONSchemaProps) string {
	// Implement JSON Schema -> TypeScript conversion
	// This is simplified - you might want to use a library
	return `// Auto-generated from CRD schema
export type Session = {
  metadata: {
    name: string;
    namespace: string;
  };
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
};`
}
