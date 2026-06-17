package api

import "embed"

//go:embed openapi.yaml
var openAPI embed.FS

var OpenAPIYAML = mustReadOpenAPI()

func mustReadOpenAPI() []byte {
	data, err := openAPI.ReadFile("openapi.yaml")
	if err != nil {
		panic(err)
	}

	return data
}
