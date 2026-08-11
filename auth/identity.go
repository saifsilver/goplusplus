package auth

import (
	"errors"
	"strconv"
	"strings"
	"unicode"

	gpp "github.com/saifsilver/goplusplus"
)

const (
	identityClaimsKey  = "user"
	identityUserIDKey  = "user_id"
	identitySubjectKey = "sub"
)

func installVerifiedIdentity(c *gpp.Context, claims UserClaims) error {
	verified, userID, numeric, err := canonicalUserClaims(claims)
	if err != nil || c == nil {
		return errors.New("auth: invalid verified identity")
	}
	clearVerifiedIdentity(c)
	c.Set(identityClaimsKey, &verified)
	if numeric {
		c.Set(identityUserIDKey, userID)
	} else {
		c.Set(identityUserIDKey, verified.ID)
	}
	c.Set(identitySubjectKey, verified.Subject)
	return nil
}

func clearVerifiedIdentity(c *gpp.Context) {
	if c == nil {
		return
	}
	c.Delete(identityClaimsKey)
	c.Delete(identityUserIDKey)
	c.Delete(identitySubjectKey)
}

func canonicalUserClaims(claims UserClaims) (UserClaims, int64, bool, error) {
	id, userID, numeric, err := canonicalIdentityID(claims.ID)
	if err != nil {
		return UserClaims{}, 0, false, err
	}
	subject := claims.Subject
	if subject == "" {
		subject = id
	}
	canonicalSubject, subjectID, subjectNumeric, err := canonicalIdentityID(subject)
	if err != nil || canonicalSubject != id || subjectNumeric != numeric || subjectID != userID {
		return UserClaims{}, 0, false, errors.New("auth: identity subject conflicts with user ID")
	}
	return UserClaims{
		ID: id, Subject: canonicalSubject, Email: claims.Email,
		Roles: append([]string(nil), claims.Roles...), Attributes: cloneStringMap(claims.Attributes), TenantID: claims.TenantID,
	}, userID, numeric, nil
}

func canonicalIdentityID(value string) (string, int64, bool, error) {
	if value == "" || len(value) > 256 {
		return "", 0, false, errors.New("auth: identity ID is invalid")
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return "", 0, false, errors.New("auth: identity ID is invalid")
		}
	}
	numericValue := strings.TrimPrefix(value, "usr_")
	if userID, err := strconv.ParseInt(numericValue, 10, 64); err == nil {
		if userID <= 0 {
			return "", 0, false, errors.New("auth: numeric identity ID must be positive")
		}
		return strconv.FormatInt(userID, 10), userID, true, nil
	}
	return value, 0, false, nil
}
