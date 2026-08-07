package swagger

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var openAPISpec []byte

// ServeSpec serves the embedded OpenAPI spec at GET /api/v1/openapi.json.
func ServeSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPISpec)
}
