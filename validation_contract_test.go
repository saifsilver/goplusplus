package gpp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
)

func TestValidationContractBoundariesAndJSONNames(t *testing.T) {
	type request struct {
		Password string `json:"password" validate:"required,min=6,max=128"`
		Role     string `json:"role" validate:"oneof=admin editor"`
	}

	tests := []struct {
		name      string
		password  string
		wantError bool
	}{
		{name: "five", password: "12345", wantError: true},
		{name: "six", password: "123456"},
		{name: "one hundred twenty eight", password: repeatedRunes('a', 128)},
		{name: "one hundred twenty nine", password: repeatedRunes('a', 129), wantError: true},
		{name: "unicode five code points", password: "😀😀😀😀😀", wantError: true},
		{name: "unicode six code points", password: "😀😀😀😀😀😀"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := (&gpp.Context{}).Validate(request{Password: test.password, Role: "admin"})
			if !test.wantError && err != nil {
				t.Fatalf("expected valid request, got %v", err)
			}
			if test.wantError {
				problem := requireValidationProblem(t, err)
				if problem.Errors[0].Field != "password" {
					t.Fatalf("expected JSON field name, got %+v", problem.Errors)
				}
			}
		})
	}
}

func TestValidationCollectsErrorsAndHandlesEmptyCollections(t *testing.T) {
	type nested struct {
		Email string `json:"email" validate:"required,email"`
	}
	type request struct {
		Name     string   `json:"name" validate:"required,min=3"`
		Tags     []string `json:"tags" validate:"required"`
		Optional []string `json:"optional" validate:"omitempty,dive,min=2"`
		Items    []nested `json:"items"`
	}

	payload := request{
		Tags: []string{}, Optional: []string{}, Items: []nested{{Email: ""}, {Email: "bad"}},
	}
	problem := requireValidationProblem(t, (&gpp.Context{}).Validate(payload))
	wantFields := []string{"name", "name", "tags", "items[0].email", "items[1].email"}
	if len(problem.Errors) != len(wantFields) {
		t.Fatalf("expected %d errors, got %+v", len(wantFields), problem.Errors)
	}
	for index, field := range wantFields {
		if problem.Errors[index].Field != field {
			t.Fatalf("error %d: expected field %q, got %+v", index, field, problem.Errors[index])
		}
	}
}

func TestValidationCycleDepthAndConcurrentCache(t *testing.T) {
	type node struct {
		Name string `json:"name" validate:"required"`
		Next *node  `json:"next"`
	}

	cycle := &node{Name: "root"}
	cycle.Next = cycle
	if err := (&gpp.Context{}).Validate(cycle); err != nil {
		t.Fatalf("cycle should terminate safely, got %v", err)
	}

	root := &node{Name: "root"}
	cursor := root
	for index := 0; index < 40; index++ {
		cursor.Next = &node{Name: "child"}
		cursor = cursor.Next
	}
	problem := requireValidationProblem(t, (&gpp.Context{}).Validate(root))
	foundDepth := false
	for _, violation := range problem.Errors {
		foundDepth = foundDepth || violation.Rule == "max_depth"
	}
	if !foundDepth {
		t.Fatalf("expected bounded-depth violation, got %+v", problem.Errors)
	}

	var wait sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for iteration := 0; iteration < 100; iteration++ {
				if err := (&gpp.Context{}).Validate(node{Name: "valid"}); err != nil {
					t.Errorf("concurrent validation failed: %v", err)
					return
				}
			}
		}()
	}
	wait.Wait()
}

func TestValidationRejectsWrongRuleType(t *testing.T) {
	invalid := struct {
		Count int `validate:"email"`
	}{Count: 1}
	problem, ok := (&gpp.Context{}).Validate(invalid).(*gpp.ProblemDetails)
	if !ok || problem.Status != 500 || problem.Detail != "Invalid validation configuration" {
		t.Fatalf("expected deterministic configuration error, got %+v", problem)
	}
}

func TestValidationRejectsInvalidNumericParameter(t *testing.T) {
	invalid := struct {
		Name string `validate:"min=not-a-number"`
	}{Name: "value"}
	problem, ok := (&gpp.Context{}).Validate(invalid).(*gpp.ProblemDetails)
	if !ok || problem.Status != 500 || problem.Detail != "Invalid validation configuration" {
		t.Fatalf("expected deterministic parameter configuration error, got %+v", problem)
	}
}

func TestBindAndValidateReturnsStableRFC7807Shape(t *testing.T) {
	app := gpp.New()
	app.POST("/register", func(c *gpp.Context) error {
		var request struct {
			Password string `json:"password" validate:"required,min=6,max=128"`
		}
		if err := c.BindAndValidate(&request); err != nil {
			return err
		}
		return c.NoContent()
	})
	request := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(`{"password":"12345"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	var problem gpp.ProblemDetails
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode validation response: %v", err)
	}
	if response.Code != 400 || problem.Instance != "/register" || problem.Detail != "One or more fields are invalid" ||
		len(problem.Errors) != 1 || problem.Errors[0].Field != "password" || problem.Errors[0].Rule != "min" {
		t.Fatalf("unexpected validation response: %+v", problem)
	}
}

func requireValidationProblem(t *testing.T, err error) *gpp.ProblemDetails {
	t.Helper()
	problem, ok := err.(*gpp.ProblemDetails)
	if !ok || problem.Type != "https://goplusplus.dev/errors/validation" || problem.Status != 400 || len(problem.Errors) == 0 {
		t.Fatalf("expected structured validation problem, got %T: %v", err, err)
	}
	return problem
}

func repeatedRunes(character rune, count int) string {
	runes := make([]rune, count)
	for index := range runes {
		runes[index] = character
	}
	return string(runes)
}
