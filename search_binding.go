package gpp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/saifsilver/goplusplus/search"
)

func BindSearchResource(group *RouterGroup, relativePath string, resource *search.Resource) {
	if group == nil || resource == nil {
		panic("gpp: search route requires a router group and resource")
	}
	group.POST(relativePath, func(c *Context) error {
		var request search.SearchRequest
		if err := c.BindJSON(&request); err != nil {
			return err
		}
		if _, err := resource.Schema().NormalizeRequest(request); err != nil {
			return ErrBadRequest(err.Error())
		}
		result, err := resource.Search(c.Request.Context(), request)
		if err != nil {
			return ErrInternal("search execution failed")
		}
		return c.JSON(http.StatusOK, result)
	})
}

func BindSearchGraphQL(engine *Engine, fieldName string, registry *search.Registry) error {
	if registry == nil {
		return fmt.Errorf("gpp: search registry is required")
	}
	types := newSearchGraphQLTypes()
	field := &graphql.Field{
		Type:        graphql.NewNonNull(types.result),
		Description: "Searches a registered resource using typed filters, facets, sorting, and cursor pagination.",
		Args: graphql.FieldConfigArgument{
			"resource": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
			"request":  &graphql.ArgumentConfig{Type: graphql.NewNonNull(types.request)},
		},
		Resolve: func(params graphql.ResolveParams) (any, error) {
			request, err := decodeGraphQLSearchRequest(params.Args["request"])
			if err != nil {
				return nil, err
			}
			result, err := registry.Search(params.Context, params.Args["resource"].(string), request)
			if err != nil {
				return nil, err
			}
			return graphQLSearchResult(result), nil
		},
	}
	return engine.RegisterGraphQLQuery(fieldName, field)
}

type searchGraphQLTypes struct {
	request *graphql.InputObject
	result  *graphql.Object
}

func newSearchGraphQLTypes() searchGraphQLTypes {
	jsonScalar := newJSONScalar()
	filterInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "SearchFilterInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"field":    &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"operator": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"value":    &graphql.InputObjectFieldConfig{Type: jsonScalar},
		},
	})
	facetInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "SearchFacetInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"field":          &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"mode":           &graphql.InputObjectFieldConfig{Type: graphql.String},
			"limit":          &graphql.InputObjectFieldConfig{Type: graphql.Int},
			"sort":           &graphql.InputObjectFieldConfig{Type: graphql.String},
			"includeMissing": &graphql.InputObjectFieldConfig{Type: graphql.Boolean},
		},
	})
	sortInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "SearchSortInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"field":     &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
			"direction": &graphql.InputObjectFieldConfig{Type: graphql.String},
		},
	})
	requestInput := graphql.NewInputObject(graphql.InputObjectConfig{
		Name: "SearchRequestInput",
		Fields: graphql.InputObjectConfigFieldMap{
			"query":   &graphql.InputObjectFieldConfig{Type: graphql.String},
			"filters": &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(filterInput))},
			"facets":  &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(facetInput))},
			"sort":    &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.NewNonNull(sortInput))},
			"cursor":  &graphql.InputObjectFieldConfig{Type: graphql.String},
			"limit":   &graphql.InputObjectFieldConfig{Type: graphql.Int},
		},
	})
	return searchGraphQLTypes{request: requestInput, result: newSearchResultType(jsonScalar)}
}

func newSearchResultType(jsonScalar *graphql.Scalar) *graphql.Object {
	longScalar := newLongScalar()
	bucketType := graphql.NewObject(graphql.ObjectConfig{
		Name: "SearchFacetBucket",
		Fields: graphql.Fields{
			"value": &graphql.Field{Type: jsonScalar},
			"count": &graphql.Field{Type: graphql.NewNonNull(longScalar)},
		},
	})
	facetType := graphql.NewObject(graphql.ObjectConfig{
		Name: "SearchFacet",
		Fields: graphql.Fields{
			"field":   &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"buckets": &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(bucketType)))},
		},
	})
	hitType := graphql.NewObject(graphql.ObjectConfig{
		Name: "SearchHit",
		Fields: graphql.Fields{
			"id":         &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
			"score":      &graphql.Field{Type: graphql.NewNonNull(graphql.Float)},
			"attributes": &graphql.Field{Type: graphql.NewNonNull(jsonScalar)},
		},
	})
	return graphql.NewObject(graphql.ObjectConfig{
		Name: "SearchResult",
		Fields: graphql.Fields{
			"items":      &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(hitType)))},
			"total":      &graphql.Field{Type: graphql.NewNonNull(longScalar)},
			"facets":     &graphql.Field{Type: graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(facetType)))},
			"nextCursor": &graphql.Field{Type: graphql.String},
			"backend":    &graphql.Field{Type: graphql.NewNonNull(graphql.String)},
		},
	})
}

func newLongScalar() *graphql.Scalar {
	return graphql.NewScalar(graphql.ScalarConfig{
		Name:        "Long",
		Description: "A signed 64-bit integer.",
		Serialize: func(value any) any {
			return value
		},
		ParseValue: func(value any) any {
			return value
		},
		ParseLiteral: func(valueAST ast.Value) any {
			parsed, _ := strconv.ParseInt(fmt.Sprint(valueAST.GetValue()), 10, 64)
			return parsed
		},
	})
}

func newJSONScalar() *graphql.Scalar {
	return graphql.NewScalar(graphql.ScalarConfig{
		Name:        "JSON",
		Description: "Arbitrary JSON used for dynamic attribute values.",
		Serialize: func(value any) any {
			return value
		},
		ParseValue: func(value any) any {
			return value
		},
		ParseLiteral: func(valueAST ast.Value) any {
			return parseJSONLiteral(valueAST)
		},
	})
}

func parseJSONLiteral(value ast.Value) any {
	switch typed := value.(type) {
	case *ast.StringValue:
		return typed.Value
	case *ast.BooleanValue:
		return typed.Value
	case *ast.IntValue:
		parsed, _ := strconv.ParseInt(typed.Value, 10, 64)
		return parsed
	case *ast.FloatValue:
		parsed, _ := strconv.ParseFloat(typed.Value, 64)
		return parsed
	case *ast.EnumValue:
		return typed.Value
	case *ast.ListValue:
		values := make([]any, 0, len(typed.Values))
		for _, item := range typed.Values {
			values = append(values, parseJSONLiteral(item))
		}
		return values
	case *ast.ObjectValue:
		object := make(map[string]any, len(typed.Fields))
		for _, field := range typed.Fields {
			object[field.Name.Value] = parseJSONLiteral(field.Value)
		}
		return object
	default:
		return value.GetValue()
	}
}

func decodeGraphQLSearchRequest(value any) (search.SearchRequest, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return search.SearchRequest{}, fmt.Errorf("gpp: encode GraphQL search request: %w", err)
	}
	var request search.SearchRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return search.SearchRequest{}, fmt.Errorf("gpp: decode GraphQL search request: %w", err)
	}
	return request, nil
}

func graphQLSearchResult(result search.SearchResult) map[string]any {
	fields := make([]string, 0, len(result.Facets))
	for field := range result.Facets {
		fields = append(fields, field)
	}
	slices.Sort(fields)
	facets := make([]search.FacetResult, 0, len(fields))
	for _, field := range fields {
		facets = append(facets, result.Facets[field])
	}
	return map[string]any{
		"items": result.Items, "total": result.Total, "facets": facets,
		"nextCursor": result.NextCursor, "backend": result.Backend,
	}
}
