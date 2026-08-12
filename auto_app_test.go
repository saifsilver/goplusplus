package gpp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gpp "github.com/saifsilver/goplusplus"
	"github.com/saifsilver/goplusplus/auth"
)

type AutoTestItem struct {
	ID        int64     `json:"id" db:"id,pk,auto_id"`
	UserID    int64     `json:"user_id" db:"user_id"`
	Title     string    `json:"title" validate:"required"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

func TestAutoAppAndAuthFlow(t *testing.T) {
	app := gpp.NewApp(gpp.AppConfig{
		DBDriver:    "sqlite",
		RWDSN:       ":memory:",
		AutoMigrate: true,
	})

	tokens, err := auth.Enable(app, auth.Config{
		Secret: "super-secret-test-key-32b-length!",
	})
	if err != nil {
		t.Fatalf("EnableAuth failed: %v", err)
	}
	if tokens == nil {
		t.Fatalf("Expected token manager to be non-nil")
	}

	if err := app.RegisterModel(t.Context(), &AutoTestItem{}); err != nil {
		t.Fatalf("RegisterModel failed: %v", err)
	}

	gpp.BindUserResource[AutoTestItem](app, "/items")

	// 1. Test Register
	regPayload, _ := json.Marshal(auth.RegisterRequest{
		Email:    "jane@example.com",
		Password: "password123",
		Name:     "Jane Dev",
	})
	reqReg := httptest.NewRequest(http.MethodPost, "/api/auth/register", bytes.NewReader(regPayload))
	reqReg.Header.Set("Content-Type", "application/json")
	wReg := httptest.NewRecorder()
	app.ServeHTTP(wReg, reqReg)

	if wReg.Code != http.StatusCreated {
		t.Fatalf("Register failed: %d - %s", wReg.Code, wReg.Body.String())
	}

	var authResp auth.Response
	if err := json.Unmarshal(wReg.Body.Bytes(), &authResp); err != nil {
		t.Fatalf("Unmarshal auth response failed: %v", err)
	}
	if authResp.Token == "" || authResp.User == nil {
		t.Fatalf("Invalid auth response: %+v", authResp)
	}

	// 2. Test Login
	loginPayload, _ := json.Marshal(auth.LoginRequest{
		Email:    "jane@example.com",
		Password: "password123",
	})
	reqLogin := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginPayload))
	reqLogin.Header.Set("Content-Type", "application/json")
	wLogin := httptest.NewRecorder()
	app.ServeHTTP(wLogin, reqLogin)

	if wLogin.Code != http.StatusOK {
		t.Fatalf("Login failed: %d - %s", wLogin.Code, wLogin.Body.String())
	}

	// 3. Test Create Item via BindUserResource
	itemPayload, _ := json.Marshal(map[string]string{
		"title": "Build ultra-fast app",
	})
	reqItem := httptest.NewRequest(http.MethodPost, "/items", bytes.NewReader(itemPayload))
	reqItem.Header.Set("Content-Type", "application/json")
	reqItem.Header.Set("Authorization", "Bearer "+authResp.Token)
	wItem := httptest.NewRecorder()
	app.ServeHTTP(wItem, reqItem)

	if wItem.Code != http.StatusCreated {
		t.Fatalf("Create user resource item failed: %d - %s", wItem.Code, wItem.Body.String())
	}

	// 4. Test List Items via BindUserResource
	reqList := httptest.NewRequest(http.MethodGet, "/items", nil)
	reqList.Header.Set("Authorization", "Bearer "+authResp.Token)
	wList := httptest.NewRecorder()
	app.ServeHTTP(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("List user resource items failed: %d - %s", wList.Code, wList.Body.String())
	}

	var listResp struct {
		Data []AutoTestItem `json:"data"`
	}
	if err := json.Unmarshal(wList.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("Unmarshal list response failed: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].Title != "Build ultra-fast app" {
		t.Fatalf("Unexpected list items response: %+v", listResp)
	}
}
