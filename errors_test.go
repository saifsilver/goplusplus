package gpp_test

import (
	"net/http"
	"testing"

	gpp "github.com/saifsilver/goplusplus"
)

func TestProblemDetailsRFC7807(t *testing.T) {
	err404 := gpp.ErrNotFound("User 42 not found")
	if err404.Status != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", err404.Status)
	}

	err400 := gpp.ErrBadRequest("Invalid email")
	if err400.Status != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", err400.Status)
	}

	err401 := gpp.ErrUnauthorized("Token expired")
	if err401.Status != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", err401.Status)
	}

	err403 := gpp.ErrForbidden("Access denied")
	if err403.Status != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", err403.Status)
	}

	err409 := gpp.ErrConflict("Email already exists")
	if err409.Status != http.StatusConflict {
		t.Errorf("expected status 409, got %d", err409.Status)
	}

	err500 := gpp.ErrInternal("DB failure")
	if err500.Status != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", err500.Status)
	}

	if err404.Error() != "HTTP 404 [Resource Not Found]: User 42 not found" {
		t.Errorf("unexpected Error() string: %s", err404.Error())
	}
}
