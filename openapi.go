package gpp

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// RouteMetadata holds parameter and summary metadata for an endpoint.
type RouteMetadata struct {
	Method      string
	Path        string
	Summary     string
	Description string
	Params      []string
}

// OpenAPIGenerator automatically inspects routes and constructs live OpenAPI 3.0 specs.
type OpenAPIGenerator struct {
	mu     sync.RWMutex
	routes []RouteMetadata
	Title  string
	Version string
}

// newOpenAPIGenerator initializes a new generator instance.
func newOpenAPIGenerator() *OpenAPIGenerator {
	return &OpenAPIGenerator{
		Title:   "goplusplus Application API",
		Version: "1.0.0",
	}
}

// RegisterRoute registers route metadata into the OpenAPI specification tree.
func (g *OpenAPIGenerator) RegisterRoute(method, pathStr string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Extract path parameter names (e.g., /users/:id -> id)
	var params []string
	parts := strings.Split(pathStr, "/")
	for _, p := range parts {
		if strings.HasPrefix(p, ":") {
			params = append(params, p[1:])
		} else if strings.HasPrefix(p, "*") {
			params = append(params, p[1:])
		}
	}

	summary := fmt.Sprintf("%s %s", method, pathStr)
	g.routes = append(g.routes, RouteMetadata{
		Method:  method,
		Path:    pathStr,
		Summary: summary,
		Params:  params,
	})
}

// GenerateJSON builds a JSON OpenAPI 3.0 specification string.
func (g *OpenAPIGenerator) GenerateJSON() string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var pathsJSON []string

	// Group routes by path template (converting :id to {id})
	pathMap := make(map[string][]RouteMetadata)
	for _, r := range g.routes {
		swaggerPath := r.Path
		parts := strings.Split(swaggerPath, "/")
		for i, p := range parts {
			if strings.HasPrefix(p, ":") {
				parts[i] = "{" + p[1:] + "}"
			} else if strings.HasPrefix(p, "*") {
				parts[i] = "{" + p[1:] + "}"
			}
		}
		swaggerPath = strings.Join(parts, "/")
		pathMap[swaggerPath] = append(pathMap[swaggerPath], r)
	}

	for p, routeList := range pathMap {
		var methodsJSON []string
		for _, r := range routeList {
			mLower := strings.ToLower(r.Method)

			var paramListJSON []string
			for _, paramName := range r.Params {
				paramListJSON = append(paramListJSON, fmt.Sprintf(`{
            "name": "%s",
            "in": "path",
            "required": true,
            "schema": { "type": "string" }
          }`, paramName))
			}

			paramStr := ""
			if len(paramListJSON) > 0 {
				paramStr = fmt.Sprintf(`"parameters": [%s],`, strings.Join(paramListJSON, ","))
			}

			methodsJSON = append(methodsJSON, fmt.Sprintf(`
      "%s": {
        "summary": "%s",
        %s
        "responses": {
          "200": {
            "description": "Successful operation",
            "content": {
              "application/json": {
                "schema": { "type": "object" }
              }
            }
          }
        }
      }`, mLower, r.Summary, paramStr))
		}

		pathsJSON = append(pathsJSON, fmt.Sprintf(`"%s": { %s }`, p, strings.Join(methodsJSON, ",")))
	}

	return fmt.Sprintf(`{
  "openapi": "3.0.0",
  "info": {
    "title": "%s",
    "version": "%s",
    "description": "Auto-generated OpenAPI specification by goplusplus framework"
  },
  "paths": {
    %s
  }
}`, g.Title, g.Version, strings.Join(pathsJSON, ","))
}

// AutoSwaggerUI returns a HandlerFunc that serves Swagger UI using the auto-generated OpenAPI spec.
func (engine *Engine) AutoSwaggerUI() HandlerFunc {
	return func(c *Context) error {
		specJSON := engine.openapi.GenerateJSON()
		htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>goplusplus API Documentation - Auto Swagger UI</title>
  <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.11.0/swagger-ui.min.css" />
  <style>
    html { box-sizing: border-box; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin:0; background: #fafafa; }
    .swagger-ui .topbar { display: none; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.11.0/swagger-ui-bundle.js"></script>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/5.11.0/swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = function() {
      const spec = %s;
      window.ui = SwaggerUIBundle({
        spec: spec,
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        plugins: [
          SwaggerUIBundle.plugins.DownloadUrl
        ],
        layout: "StandaloneLayout"
      });
    };
  </script>
</body>
</html>`, specJSON)

		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		c.Status(http.StatusOK)
		_, err := c.Writer.Write([]byte(htmlContent))
		return err
	}
}
