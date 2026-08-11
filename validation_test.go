package gpp_test

import (
	"testing"

	"github.com/saifsilver/goplusplus"
)

type validationAddress struct {
	City string `validate:"required,min=2,max=40"`
}

type validationPayload struct {
	Username        string            `validate:"required,min=3,max=12,alphanum"`
	Age             int               `validate:"gte=18,lte=120"`
	Role            string            `validate:"oneof=admin editor"`
	Email           string            `validate:"required,email"`
	Website         string            `validate:"omitempty,url"`
	RequestID       string            `validate:"uuid4"`
	Tags            []string          `validate:"min=1,max=3,dive,required,lowercase"`
	Password        string            `validate:"min=8"`
	ConfirmPassword string            `validate:"eqfield=Password"`
	Address         validationAddress `validate:"required"`
}

func validValidationPayload() validationPayload {
	return validationPayload{
		Username:        "alice123",
		Age:             30,
		Role:            "admin",
		Email:           "alice@example.com",
		Website:         "https://example.com/profile",
		RequestID:       "550e8400-e29b-41d4-a716-446655440000",
		Tags:            []string{"go", "api"},
		Password:        "correct-horse",
		ConfirmPassword: "correct-horse",
		Address:         validationAddress{City: "Pune"},
	}
}

func TestValidateSupportsStandardValidationRules(t *testing.T) {
	ctx := &gpp.Context{}

	if err := ctx.Validate(validValidationPayload()); err != nil {
		t.Fatalf("expected payload to pass validation, got %v", err)
	}
}

func TestValidateRejectsTypeAwareBounds(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*validationPayload)
		field   string
		rule    string
		message string
	}{
		{
			name: "string minimum",
			mutate: func(payload *validationPayload) {
				payload.Username = "ab"
			},
			field: "Username", rule: "min", message: "must contain at least 3 characters",
		},
		{
			name: "string maximum",
			mutate: func(payload *validationPayload) {
				payload.Username = "abcdefghijkl3"
			},
			field: "Username", rule: "max", message: "must contain at most 12 characters",
		},
		{
			name: "numeric minimum",
			mutate: func(payload *validationPayload) {
				payload.Age = 17
			},
			field: "Age", rule: "gte", message: "failed validation rule 'gte=18'",
		},
		{
			name: "collection maximum",
			mutate: func(payload *validationPayload) {
				payload.Tags = []string{"go", "api", "web", "http"}
			},
			field: "Tags", rule: "max", message: "must contain at most 3 items",
		},
	}

	ctx := &gpp.Context{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := validValidationPayload()
			test.mutate(&payload)

			assertViolation(t, ctx.Validate(payload), test.field, test.rule, test.message)
		})
	}
}

func TestValidateSupportsNestedDiveFormatAndCrossFieldRules(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*validationPayload)
		field  string
	}{
		{
			name: "nested field",
			mutate: func(payload *validationPayload) {
				payload.Address.City = "X"
			},
			field: "Address.City",
		},
		{
			name: "slice dive",
			mutate: func(payload *validationPayload) {
				payload.Tags = []string{"Go"}
			},
			field: "Tags[0]",
		},
		{
			name: "format",
			mutate: func(payload *validationPayload) {
				payload.RequestID = "not-a-uuid"
			},
			field: "RequestID",
		},
		{
			name: "cross field",
			mutate: func(payload *validationPayload) {
				payload.ConfirmPassword = "different-value"
			},
			field: "ConfirmPassword",
		},
	}

	ctx := &gpp.Context{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := validValidationPayload()
			test.mutate(&payload)

			err := ctx.Validate(payload)
			problem, ok := err.(*gpp.ProblemDetails)
			if !ok || problem.Status != 400 {
				t.Fatalf("expected 400 problem details, got %v", err)
			}
			if len(problem.Errors) == 0 || problem.Errors[0].Field != test.field {
				t.Fatalf("expected field %q in errors, got %+v", test.field, problem.Errors)
			}
		})
	}
}

func TestValidatePreservesOptionalAndNonStructBehavior(t *testing.T) {
	ctx := &gpp.Context{}
	payload := validValidationPayload()
	payload.Website = ""

	if err := ctx.Validate(payload); err != nil {
		t.Fatalf("expected omitempty field to pass, got %v", err)
	}
	if err := ctx.Validate("not a struct"); err != nil {
		t.Fatalf("expected non-struct target to remain a no-op, got %v", err)
	}
	optionalEmail := struct {
		Email string `validate:"email"`
	}{}
	if err := ctx.Validate(optionalEmail); err != nil {
		t.Fatalf("expected optional email behavior to remain compatible, got %v", err)
	}
}

func TestValidateSupportsConditionalAndMapRules(t *testing.T) {
	type conditionalPayload struct {
		Mode      string            `validate:"oneof=relaxed strict"`
		Approval  string            `validate:"required_if=Mode strict"`
		DebugData string            `validate:"excluded_if=Mode strict"`
		Aliases   []string          `validate:"unique"`
		Labels    map[string]string `validate:"dive,keys,lowercase,endkeys,required"`
	}

	ctx := &gpp.Context{}
	valid := conditionalPayload{
		Mode: "strict", Approval: "approved", Aliases: []string{"one", "two"},
		Labels: map[string]string{"region": "ap-south"},
	}
	if err := ctx.Validate(valid); err != nil {
		t.Fatalf("expected conditional payload to pass, got %v", err)
	}

	missingApproval := valid
	missingApproval.Approval = ""
	assertViolation(t, ctx.Validate(missingApproval), "Approval", "required_if", "is required")

	excludedDebugData := valid
	excludedDebugData.DebugData = "sensitive"
	assertViolation(t, ctx.Validate(excludedDebugData), "DebugData", "excluded_if", "failed validation rule 'excluded_if=Mode strict'")

	duplicateAliases := valid
	duplicateAliases.Aliases = []string{"same", "same"}
	assertViolation(t, ctx.Validate(duplicateAliases), "Aliases", "unique", "failed validation rule 'unique'")

	invalidMapKey := valid
	invalidMapKey.Labels = map[string]string{"Region": "ap-south"}
	err := ctx.Validate(invalidMapKey)
	problem, ok := err.(*gpp.ProblemDetails)
	if !ok || len(problem.Errors) == 0 || problem.Errors[0].Field != "Labels[0].key" {
		t.Fatalf("expected deterministic map-key error, got %v", err)
	}
}

func TestValidateSupportsBuiltInTextAndFormatRules(t *testing.T) {
	type builtInPayload struct {
		Alpha        string `validate:"alpha"`
		AlphaNum     string `validate:"alphanum"`
		Numeric      string `validate:"numeric"`
		Number       string `validate:"number"`
		Lower        string `validate:"lowercase"`
		Upper        string `validate:"uppercase"`
		ASCII        string `validate:"ascii"`
		Printable    string `validate:"printascii"`
		Boolean      string `validate:"boolean"`
		Relations    string `validate:"contains=plus,containsany=xyz,containsrune=+,excludes=minus,excludesall=ABC,excludesrune=!,startswith=go,endswith=xyz"`
		HTTPURL      string `validate:"http_url"`
		IP           string `validate:"ip"`
		IPv4         string `validate:"ipv4"`
		IPv6         string `validate:"ipv6"`
		CIDR         string `validate:"cidr"`
		CIDRv4       string `validate:"cidrv4"`
		CIDRv6       string `validate:"cidrv6"`
		Hostname     string `validate:"hostname"`
		HostnamePort string `validate:"hostname_port"`
		UUID         string `validate:"uuid"`
		UUID3        string `validate:"uuid3"`
		UUID5        string `validate:"uuid5"`
		JSON         string `validate:"json"`
		Base64       string `validate:"base64"`
		Base64URL    string `validate:"base64url"`
		Hex          string `validate:"hexadecimal"`
		HexColor     string `validate:"hexcolor"`
		Date         string `validate:"datetime=2006-01-02"`
	}

	payload := builtInPayload{
		Alpha: "Golang", AlphaNum: "Go123", Numeric: "123", Number: "-12.5e2",
		Lower: "lower", Upper: "UPPER", ASCII: "ascii", Printable: "hello!", Boolean: "true",
		Relations: "goplus+xyz", HTTPURL: "https://example.com/path", IP: "203.0.113.1",
		IPv4: "192.0.2.1", IPv6: "2001:db8::1", CIDR: "10.0.0.0/8", CIDRv4: "192.0.2.0/24",
		CIDRv6: "2001:db8::/32", Hostname: "api.example.com", HostnamePort: "api.example.com:443",
		UUID: "550e8400-e29b-41d4-a716-446655440000", UUID3: "550e8400-e29b-31d4-a716-446655440000",
		UUID5: "550e8400-e29b-51d4-a716-446655440000", JSON: `{"ok":true}`, Base64: "aGVsbG8=",
		Base64URL: "aGVsbG8", Hex: "deadbeef", HexColor: "#1a2b3c", Date: "2026-08-11",
	}

	if err := (&gpp.Context{}).Validate(payload); err != nil {
		t.Fatalf("expected all built-in text and format rules to pass, got %v", err)
	}
}

func TestValidateSupportsComparisonRules(t *testing.T) {
	type comparisonPayload struct {
		Exact    string `validate:"len=4,eq=test,ne=fail"`
		Choice   string `validate:"not_oneof=root superuser"`
		Score    int    `validate:"gt=0,gte=10,lt=20,lte=10"`
		Minimum  int
		Maximum  int `validate:"gtfield=Minimum"`
		Same     int `validate:"eqfield=Minimum"`
		NotSame  int `validate:"nefield=Minimum"`
		AtLeast  int `validate:"gtefield=Minimum"`
		LessThan int `validate:"ltfield=Maximum"`
		AtMost   int `validate:"ltefield=Maximum"`
	}

	payload := comparisonPayload{
		Exact: "test", Choice: "member", Score: 10, Minimum: 5, Maximum: 10,
		Same: 5, NotSame: 6, AtLeast: 5, LessThan: 9, AtMost: 10,
	}
	if err := (&gpp.Context{}).Validate(payload); err != nil {
		t.Fatalf("expected comparison rules to pass, got %v", err)
	}
}

func TestValidateRejectsNilAndInvalidValidationConfiguration(t *testing.T) {
	ctx := &gpp.Context{}
	var typedNil *validationPayload

	assertViolation(t, ctx.Validate(nil), "request", "required", "must not be nil")
	assertViolation(t, ctx.Validate(typedNil), "request", "required", "must not be nil")

	invalid := struct {
		Value string `validate:"unsupported_rule"`
	}{Value: "value"}
	assertProblemDetail(t, ctx.Validate(invalid), 500, "Invalid validation configuration")
}

func assertProblemDetail(t *testing.T, err error, status int, detail string) {
	t.Helper()
	problem, ok := err.(*gpp.ProblemDetails)
	if !ok {
		t.Fatalf("expected problem details, got %T: %v", err, err)
	}
	if problem.Status != status || problem.Detail != detail {
		t.Fatalf("expected status %d and detail %q, got status %d and detail %q", status, detail, problem.Status, problem.Detail)
	}
}

func assertViolation(t *testing.T, err error, field, rule, message string) {
	t.Helper()
	problem, ok := err.(*gpp.ProblemDetails)
	if !ok || problem.Status != 400 || problem.Type != "https://goplusplus.dev/errors/validation" {
		t.Fatalf("expected validation problem, got %T: %v", err, err)
	}
	for _, violation := range problem.Errors {
		if violation.Field == field && violation.Rule == rule && violation.Message == message {
			return
		}
	}
	t.Fatalf("expected violation %s/%s/%s, got %+v", field, rule, message, problem.Errors)
}
