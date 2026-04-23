package controlapi

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var openAPISpecYAML []byte

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Softprobe Runtime API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" crossorigin="anonymous" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js" crossorigin="anonymous"></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        url: new URL("/openapi.yaml", window.location.origin).href,
        dom_id: '#swagger-ui',
        persistAuthorization: true,
      });
    };
  </script>
</body>
</html>`

func registerAPIDocs(mux *http.ServeMux) {
	mux.HandleFunc("/openapi.yaml", serveOpenAPIYAML)
	mux.HandleFunc("/docs", serveSwaggerUI)
	mux.HandleFunc("/docs/", serveSwaggerUI)
}

func serveOpenAPIYAML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowedError(w)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPISpecYAML)
}

func serveSwaggerUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowedError(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerUIHTML))
}
