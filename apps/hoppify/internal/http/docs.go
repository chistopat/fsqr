package httpapi

import (
	"encoding/json"
	"net/http"

	openapi "github.com/chistopat/hoppify/api"

	"go.yaml.in/yaml/v3"
)

const swaggerViewerHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Hoppify API docs</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
  <style>
    body { margin: 0; background: #f7f7f7; }
    #swagger-ui { max-width: 1440px; margin: 0 auto; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function() {
      window.ui = SwaggerUIBundle({
        url: "/swagger.json",
        dom_id: "#swagger-ui",
        presets: [SwaggerUIBundle.presets.apis],
        layout: "BaseLayout"
      });
    };
  </script>
</body>
</html>
`

var swaggerJSON = mustBuildSwaggerJSON()

func serveSwaggerJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(swaggerJSON)
}

func serveSwaggerViewer(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerViewerHTML))
}

func mustBuildSwaggerJSON() []byte {
	var document any
	if err := yaml.Unmarshal(openapi.OpenAPIYAML, &document); err != nil {
		panic(err)
	}

	data, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}

	return data
}
