package api

import _ "embed"

// OpenAPIYAML is the service OpenAPI document embedded into the binary.
//
//go:embed openapi.yaml
var OpenAPIYAML []byte
