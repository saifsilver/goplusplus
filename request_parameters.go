package gpp

import (
	"math"
	"net/http"
	"strconv"
	"strings"
)

func (c *Context) ParamIntStrict(key string) (int, error) {
	value, err := c.strictParameter(key, c.Param(key), c.hasParam(key), strconv.IntSize)
	return int(value), err
}

func (c *Context) ParamInt64Strict(key string) (int64, error) {
	return c.strictParameter(key, c.Param(key), c.hasParam(key), 64)
}

func (c *Context) ParamPositiveInt64(key string) (int64, error) {
	value, err := c.ParamInt64Strict(key)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, parameterProblem(key, "positive", "Path parameter must be positive")
	}
	return value, nil
}

func (c *Context) ParamUUIDStrict(key string) (string, error) {
	value, exists := c.Param(key), c.hasParam(key)
	if !exists {
		return "", parameterProblem(key, "required", "Path parameter is required")
	}
	if value == "" {
		return "", parameterProblem(key, "required", "Path parameter must not be empty")
	}
	if strings.TrimSpace(value) != value || !isValidUUID(value, "uuid") {
		return "", parameterProblem(key, "uuid", "Path parameter must be a valid UUID")
	}
	return value, nil
}

func (c *Context) QueryInt64Strict(key string) (int64, error) {
	values, exists := c.Request.URL.Query()[key]
	value := ""
	if exists && len(values) > 0 {
		value = values[0]
	}
	return c.strictParameter(key, value, exists, 64)
}

func (c *Context) QueryOptionalInt64Strict(key string) (*int64, error) {
	if _, exists := c.Request.URL.Query()[key]; !exists {
		return nil, nil
	}
	value, err := c.QueryInt64Strict(key)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (c *Context) hasParam(key string) bool {
	for _, parameter := range c.Params {
		if parameter.Key == key {
			return true
		}
	}
	return false
}

func (c *Context) strictParameter(key, value string, exists bool, bits int) (int64, error) {
	if !exists {
		return 0, parameterProblem(key, "required", "Parameter is required")
	}
	if value == "" {
		return 0, parameterProblem(key, "required", "Parameter must not be empty")
	}
	if strings.TrimSpace(value) != value {
		return 0, parameterProblem(key, "integer", "Parameter must be an integer")
	}
	parsed, err := strconv.ParseInt(value, 10, bits)
	if err != nil {
		return 0, parameterProblem(key, "integer", "Parameter must be an integer in range")
	}
	return parsed, nil
}

func parameterProblem(field, rule, message string) *ProblemDetails {
	problem := ErrValidation([]FieldViolation{{Field: field, Rule: rule, Message: message}})
	problem.Status = http.StatusBadRequest
	return problem
}

type PaginationPolicy struct {
	DefaultPage    int
	DefaultLimit   int
	MaximumLimit   int
	MaximumPage    int
	PageParameter  string
	LimitParameter string
	Strict         bool
}

type Pagination struct {
	Page   int `json:"page"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

func (policy PaginationPolicy) Parse(c *Context) (Pagination, error) {
	policy, err := normalizePaginationPolicy(policy)
	if err != nil {
		return Pagination{}, err
	}
	page, err := parsePaginationValue(c, policy.PageParameter, policy.DefaultPage, policy.Strict)
	if err != nil {
		return Pagination{}, err
	}
	limit, err := parsePaginationValue(c, policy.LimitParameter, policy.DefaultLimit, policy.Strict)
	if err != nil {
		return Pagination{}, err
	}
	if page > policy.MaximumPage {
		if policy.Strict {
			return Pagination{}, parameterProblem(policy.PageParameter, "maximum", "Page exceeds the configured maximum")
		}
		page = policy.MaximumPage
	}
	if limit > policy.MaximumLimit {
		if policy.Strict {
			return Pagination{}, parameterProblem(policy.LimitParameter, "maximum", "Limit exceeds the configured maximum")
		}
		limit = policy.MaximumLimit
	}
	if page-1 > math.MaxInt/limit {
		return Pagination{}, parameterProblem(policy.PageParameter, "overflow", "Pagination offset is too large")
	}
	return Pagination{Page: page, Limit: limit, Offset: (page - 1) * limit}, nil
}

func normalizePaginationPolicy(policy PaginationPolicy) (PaginationPolicy, error) {
	if policy.DefaultPage == 0 {
		policy.DefaultPage = 1
	}
	if policy.DefaultLimit == 0 {
		policy.DefaultLimit = 20
	}
	if policy.MaximumLimit == 0 {
		policy.MaximumLimit = 1000
	}
	if policy.MaximumPage == 0 {
		policy.MaximumPage = math.MaxInt
	}
	if policy.PageParameter == "" {
		policy.PageParameter = "page"
	}
	if policy.LimitParameter == "" {
		policy.LimitParameter = "limit"
	}
	if policy.DefaultPage < 1 || policy.DefaultLimit < 1 || policy.MaximumLimit < policy.DefaultLimit || policy.MaximumPage < policy.DefaultPage {
		return policy, ErrInternal("Invalid pagination policy")
	}
	return policy, nil
}

func parsePaginationValue(c *Context, key string, fallback int, strict bool) (int, error) {
	values, exists := c.Request.URL.Query()[key]
	if !exists || len(values) == 0 || values[0] == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(values[0])
	if err != nil || value < 1 {
		if strict {
			return 0, parameterProblem(key, "positive_integer", "Pagination parameter must be a positive integer")
		}
		return fallback, nil
	}
	return value, nil
}

func TotalPages(total, limit int) (int, error) {
	if total < 0 || limit <= 0 {
		return 0, ErrInternal("Invalid pagination totals")
	}
	if total == 0 {
		return 0, nil
	}
	return 1 + (total-1)/limit, nil
}
