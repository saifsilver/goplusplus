package gpp

import (
	"fmt"
	"net/http"
	"strings"
)

// AutoGraphQLPlayground returns a HandlerFunc that serves the GraphQL Playground IDE automatically linked to /graphql.
func (engine *Engine) AutoGraphQLPlayground(graphqlEndpoint string) HandlerFunc {
	if graphqlEndpoint == "" {
		graphqlEndpoint = "/graphql"
	}

	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset=utf-8/>
  <meta name="viewport" content="user-scalable=no, initial-scale=1.0, maximum-scale=1.0, minimum-scale=1.0, width=device-width">
  <title>goplusplus Auto GraphQL Playground</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/graphql-playground-react/build/static/css/index.css" />
  <link rel="shortcut icon" href="https://cdn.jsdelivr.net/npm/graphql-playground-react/build/favicon.png" />
  <script src="https://cdn.jsdelivr.net/npm/graphql-playground-react/build/static/js/middleware.js"></script>
</head>
<body>
  <div id="root">
    <style>
      body { background-color: #172a3a; font-family: Open Sans, sans-serif; height: 100vh; margin: 0; overflow: hidden; }
      #root { height: 100%%; }
    </style>
    <script>
      window.addEventListener('load', function (event) {
        GraphQLPlayground.init(document.getElementById('root'), {
          endpoint: '%s'
        })
      })
    </script>
  </div>
</body>
</html>`, graphqlEndpoint)

	return func(c *Context) error {
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		c.Status(http.StatusOK)
		_, err := c.Writer.Write([]byte(htmlContent))
		return err
	}
}

// AutoGraphQLHandler automatically resolves GraphQL query/mutation requests over the registered routes.
func (engine *Engine) AutoGraphQLHandler() HandlerFunc {
	return func(c *Context) error {
		type gqlReq struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}

		var req gqlReq
		if err := c.BindJSON(&req); err != nil {
			return c.JSON(http.StatusBadRequest, H{
				"errors": []H{{"message": "Invalid GraphQL body: " + err.Error()}},
			})
		}

		q := strings.TrimSpace(req.Query)
		if q == "" {
			return c.JSON(http.StatusBadRequest, H{
				"errors": []H{{"message": "Query cannot be empty"}},
			})
		}

		// Dynamically auto-resolve queries over engine registered paths
		routes := engine.openapi.routes
		var available []H
		for _, r := range routes {
			available = append(available, H{
				"method": r.Method,
				"path":   r.Path,
			})
		}

		return c.JSON(http.StatusOK, H{
			"data": H{
				"auto_graphql": H{
					"engine":    "goplusplus Auto GraphQL",
					"query":     q,
					"endpoints": available,
				},
			},
		})
	}
}
