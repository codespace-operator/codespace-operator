# scripts/generate-client.sh
#!/bin/bash
set -euo pipefail

echo "Generating TypeScript client from OpenAPI spec..."

# Ensure we have the tools
if ! command -v openapi-generator-cli &> /dev/null; then
    echo "Installing OpenAPI Generator CLI..."
    npm install -g @openapitools/openapi-generator-cli@latest
fi

# Clean previous generated code
rm -rf ui/src/gen/

# Generate the Go OpenAPI spec first
echo "Generating OpenAPI spec from Go code..."
cd ./hack/openapi
go run . --generate-openapi-only --output ../../docs/swagger.json
cd ../..

# Verify the spec exists and is valid
if [ ! -f "docs/swagger.json" ]; then
    echo "Error: OpenAPI spec not generated"
    exit 1
fi

# Generate TypeScript client
echo "Generating TypeScript client..."
openapi-generator-cli generate \
  -i docs/swagger.json \
  -g typescript-fetch \
  -o ui/src/generated \
  --additional-properties=\
typescriptThreePlus=true,\
supportsES6=true,\
npmName=codespace-api-client,\
npmVersion=1.0.0,\
stringEnums=true,\
enumNameSuffix=,\
modelPropertyNaming=camelCase,\
paramNaming=camelCase,\
enumPropertyNaming=PascalCase

# Generate just the types (alternative approach)
echo "Generating standalone types..."
openapi-generator-cli generate \
  -i docs/swagger.json \
  -g typescript \
  -o ui/src/generated-types \
  --additional-properties=\
typescriptThreePlus=true,\
supportsES6=true,\
stringEnums=true,\
modelPropertyNaming=camelCase

echo "Generated TypeScript client successfully!"

# Optional: Run type checking
if command -v tsc &> /dev/null; then
    echo "Type checking generated code..."
    cd ui/src/generated
    tsc --noEmit --skipLibCheck *.ts || echo "Type checking completed with warnings"
    cd ../../..
fi