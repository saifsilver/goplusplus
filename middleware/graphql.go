package middleware

import (
	"fmt"
	"log/slog"
	"net/http"

	gpp "github.com/saifsilver/goplusplus"
)

// GraphQLResolver defines the contract for resolving GraphQL query and mutation requests.
type GraphQLResolver func(query string, variables map[string]any) (any, error)

// GraphQLRequest payload representing incoming GraphQL requests.
type GraphQLRequest struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName,omitempty"`
	Variables     map[string]any `json:"variables,omitempty"`
}

// GraphQLPlayground serves an interactive GraphQL Playground IDE web interface.
func GraphQLPlayground(endpoint string) gpp.HandlerFunc {
	htmlContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset=utf-8/>
  <meta name="viewport" content="user-scalable=no, initial-scale=1.0, maximum-scale=1.0, minimum-scale=1.0, width=device-width">
  <title>go++ GraphQL Playground</title>
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
</html>`, endpoint)

	return func(c *gpp.Context) error {
		return c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(htmlContent))
	}
}

// GraphQLHandler executes GraphQL query requests and returns JSON responses.
func GraphQLHandler(resolver GraphQLResolver) gpp.HandlerFunc {
	return func(c *gpp.Context) error {
		var req GraphQLRequest
		if err := c.BindJSON(&req); err != nil {
			return c.JSON(http.StatusBadRequest, gpp.H{
				"errors": []gpp.H{{"message": "Invalid GraphQL request body"}},
			})
		}

		if req.Query == "" {
			return c.JSON(http.StatusBadRequest, gpp.H{
				"errors": []gpp.H{{"message": "GraphQL query string cannot be empty"}},
			})
		}

		data, err := resolver(req.Query, req.Variables)
		if err != nil {
			slog.Error("graphql: resolver failed", slog.String("error", err.Error()), slog.String("request_id", c.RequestID()))
			return c.JSON(http.StatusOK, gpp.H{
				"data":   data,
				"errors": []gpp.H{{"message": "GraphQL operation failed"}},
			})
		}

		return c.JSON(http.StatusOK, gpp.H{
			"data": data,
		})
	}
}
