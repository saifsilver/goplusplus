package auth

import (
	"testing"
	"time"
)

func TestAuthSecuritySuite(t *testing.T) {
	secret := "super_secret_jwt_key_991823"

	// 1. Password Hashing & Verification
	hash := HashPassword("my_password123", secret)
	if hash == "" || hash == "my_password123" {
		t.Fatalf("HashPassword failed: %s", hash)
	}

	if !VerifyPassword("my_password123", secret, hash) {
		t.Errorf("VerifyPassword failed for correct password")
	}

	if VerifyPassword("wrong_password", secret, hash) {
		t.Errorf("VerifyPassword succeeded for wrong password")
	}

	// 2. Token Generation & Verification
	userID := int64(10088)
	token := GenerateToken(userID, secret, 5*time.Minute)
	if token == "" {
		t.Fatalf("GenerateToken failed")
	}

	claims, err := VerifyToken(token, secret)
	if err != nil || claims == nil {
		t.Fatalf("VerifyToken failed: %v", err)
	}

	if claims.UserID != userID {
		t.Errorf("expected claims.UserID = %d, got %d", userID, claims.UserID)
	}

	// Invalid signature test
	_, errInvalid := VerifyToken(token, "wrong_secret")
	if errInvalid == nil {
		t.Errorf("expected error for invalid secret signature")
	}

	// Expired token test
	expiredToken := GenerateToken(userID, secret, -1*time.Minute)
	_, errExpired := VerifyToken(expiredToken, secret)
	if errExpired == nil {
		t.Errorf("expected error for expired token")
	}
}
