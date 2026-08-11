package gpp

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/graphql-go/graphql"
	gqlast "github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
)

var graphQLFieldNamePattern = regexp.MustCompile(`^[_A-Za-z][_0-9A-Za-z]*$`)

const (
	maxGraphQLQueryLength = 16 << 10
	maxGraphQLFields      = 100
	maxGraphQLDepth       = 12
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

// RegisterGraphQLQuery adds a schema-validated field to the automatic GraphQL endpoint.
func (engine *Engine) RegisterGraphQLQuery(name string, field *graphql.Field) error {
	if !graphQLFieldNamePattern.MatchString(name) {
		return fmt.Errorf("gpp: invalid GraphQL field name %q", name)
	}
	if field == nil {
		return fmt.Errorf("gpp: GraphQL field %q cannot be nil", name)
	}
	engine.graphqlMu.Lock()
	defer engine.graphqlMu.Unlock()
	if _, exists := engine.graphqlFields[name]; exists {
		return fmt.Errorf("gpp: GraphQL field %q is already registered", name)
	}
	engine.graphqlFields[name] = field
	engine.graphqlSchema = nil
	return nil
}

func (engine *Engine) autoGraphQLSchema() (*graphql.Schema, error) {
	engine.graphqlMu.Lock()
	defer engine.graphqlMu.Unlock()
	if engine.graphqlSchema != nil {
		return engine.graphqlSchema, nil
	}
	fields := make(graphql.Fields, len(engine.graphqlFields)+1)
	for name, field := range engine.graphqlFields {
		fields[name] = field
	}
	fields["gpp"] = &graphql.Field{
		Type:        graphql.NewNonNull(graphql.String),
		Description: "Identifies the goplusplus GraphQL runtime.",
		Resolve: func(graphql.ResolveParams) (any, error) {
			return "goplusplus Auto GraphQL", nil
		},
	}
	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{Name: "Query", Fields: fields}),
	})
	if err != nil {
		return nil, fmt.Errorf("gpp: build GraphQL schema: %w", err)
	}
	engine.graphqlSchema = &schema
	return engine.graphqlSchema, nil
}

// AutoGraphQLHandler executes schema-validated GraphQL queries over registered fields.
func (engine *Engine) AutoGraphQLHandler() HandlerFunc {
	return func(c *Context) error {
		type gqlReq struct {
			Query     string         `json:"query"`
			Operation string         `json:"operationName,omitempty"`
			Variables map[string]any `json:"variables"`
		}

		var req gqlReq
		if err := c.BindJSON(&req); err != nil {
			return c.JSON(http.StatusBadRequest, H{
				"errors": []H{{"message": "Invalid GraphQL request body"}},
			})
		}

		q := strings.TrimSpace(req.Query)
		if q == "" {
			return c.JSON(http.StatusBadRequest, H{
				"errors": []H{{"message": "Query cannot be empty"}},
			})
		}
		if err := validateGraphQLComplexity(q); err != nil {
			return c.JSON(http.StatusBadRequest, H{
				"errors": []H{{"message": err.Error()}},
			})
		}

		schema, err := engine.autoGraphQLSchema()
		if err != nil {
			slog.Error("graphql: schema construction failed", slog.String("error", err.Error()), slog.String("request_id", c.RequestID()))
			return c.JSON(http.StatusInternalServerError, H{
				"errors": []H{{"message": "GraphQL service is unavailable"}},
			})
		}
		result := graphql.Do(graphql.Params{
			Schema:         *schema,
			RequestString:  q,
			VariableValues: req.Variables,
			OperationName:  req.Operation,
			Context:        c.Request.Context(),
		})
		if len(result.Errors) > 0 {
			for _, resultError := range result.Errors {
				slog.Error("graphql: operation failed",
					slog.String("error", resultError.Message), slog.String("request_id", c.RequestID()),
				)
			}
			return c.JSON(http.StatusOK, H{
				"data": result.Data, "errors": []H{{"message": "GraphQL operation failed"}},
			})
		}
		return c.JSON(http.StatusOK, result)
	}
}

func validateGraphQLComplexity(query string) error {
	if len(query) > maxGraphQLQueryLength {
		return fmt.Errorf("GraphQL query exceeds %d bytes", maxGraphQLQueryLength)
	}
	document, err := parser.Parse(parser.ParseParams{Source: query})
	if err != nil {
		return fmt.Errorf("invalid GraphQL query")
	}
	fragments := graphQLFragments(document)
	fieldCount := 0
	for _, definition := range document.Definitions {
		operation, ok := definition.(*gqlast.OperationDefinition)
		if !ok {
			continue
		}
		count, err := countGraphQLSelections(operation.SelectionSet, 1, fragments, make(map[string]bool))
		if err != nil {
			return err
		}
		fieldCount += count
	}
	if fieldCount > maxGraphQLFields {
		return fmt.Errorf("GraphQL query exceeds %d fields", maxGraphQLFields)
	}
	return nil
}

func graphQLFragments(document *gqlast.Document) map[string]*gqlast.FragmentDefinition {
	fragments := make(map[string]*gqlast.FragmentDefinition)
	for _, definition := range document.Definitions {
		fragment, ok := definition.(*gqlast.FragmentDefinition)
		if ok && fragment.Name != nil {
			fragments[fragment.Name.Value] = fragment
		}
	}
	return fragments
}

func countGraphQLSelections(selectionSet *gqlast.SelectionSet, depth int, fragments map[string]*gqlast.FragmentDefinition, active map[string]bool) (int, error) {
	if selectionSet == nil {
		return 0, nil
	}
	if depth > maxGraphQLDepth {
		return 0, fmt.Errorf("GraphQL query exceeds depth %d", maxGraphQLDepth)
	}
	count := 0
	for _, selection := range selectionSet.Selections {
		childCount, err := countGraphQLSelection(selection, depth, fragments, active)
		if err != nil {
			return 0, err
		}
		count += childCount
		if count > maxGraphQLFields {
			return count, nil
		}
	}
	return count, nil
}

func countGraphQLSelection(selection gqlast.Selection, depth int, fragments map[string]*gqlast.FragmentDefinition, active map[string]bool) (int, error) {
	switch typed := selection.(type) {
	case *gqlast.Field:
		children, err := countGraphQLSelections(typed.SelectionSet, depth+1, fragments, active)
		return 1 + children, err
	case *gqlast.InlineFragment:
		return countGraphQLSelections(typed.SelectionSet, depth+1, fragments, active)
	case *gqlast.FragmentSpread:
		return countGraphQLFragment(typed, depth, fragments, active)
	default:
		return 0, nil
	}
}

func countGraphQLFragment(spread *gqlast.FragmentSpread, depth int, fragments map[string]*gqlast.FragmentDefinition, active map[string]bool) (int, error) {
	if spread.Name == nil {
		return 0, nil
	}
	name := spread.Name.Value
	if active[name] {
		return 0, fmt.Errorf("GraphQL fragment cycle detected at %q", name)
	}
	fragment := fragments[name]
	if fragment == nil {
		return 0, nil
	}
	active[name] = true
	defer delete(active, name)
	return countGraphQLSelections(fragment.SelectionSet, depth+1, fragments, active)
}
