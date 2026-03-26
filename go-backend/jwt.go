package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const jwtExpireDays = 90

type JWTClaims struct {
	Sub    string `json:"sub"`
	Iat    int64  `json:"iat"`
	Exp    int64  `json:"exp"`
	User   string `json:"user"`
	Name   string `json:"name"`
	RoleID int    `json:"role_id"`
}

// GenerateToken creates a JWT token for the given user
func GenerateToken(user *User) (string, error) {
	now := time.Now()
	exp := now.Add(time.Duration(jwtExpireDays) * 24 * time.Hour)

	header := map[string]string{
		"alg": "HmacSHA256",
		"typ": "JWT",
	}
	payload := JWTClaims{
		Sub:    fmt.Sprintf("%d", user.ID),
		Iat:    now.Unix(),
		Exp:    exp.Unix(),
		User:   user.User,
		Name:   user.User,
		RoleID: user.RoleID,
	}

	headerJSON, _ := json.Marshal(header)
	payloadJSON, _ := json.Marshal(payload)

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)

	sig, err := calcSignature(encodedHeader, encodedPayload)
	if err != nil {
		return "", err
	}

	return encodedHeader + "." + encodedPayload + "." + sig, nil
}

// ValidateToken checks JWT signature and expiry
func ValidateToken(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}

	expectedSig, err := calcSignature(parts[0], parts[1])
	if err != nil || expectedSig != parts[2] {
		return false
	}

	claims, err := parsePayload(parts[1])
	if err != nil {
		return false
	}

	return claims.Exp > time.Now().Unix()
}

// GetUserIDFromToken extracts userId from JWT without re-validating
func GetUserIDFromToken(token string) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid token")
	}
	claims, err := parsePayload(parts[1])
	if err != nil {
		return 0, err
	}
	var id int64
	fmt.Sscanf(claims.Sub, "%d", &id)
	return id, nil
}

// GetRoleIDFromToken extracts roleId from JWT without re-validating
func GetRoleIDFromToken(token string) (int, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid token")
	}
	claims, err := parsePayload(parts[1])
	if err != nil {
		return 0, err
	}
	return claims.RoleID, nil
}

func parsePayload(encodedPayload string) (*JWTClaims, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return nil, err
	}
	var claims JWTClaims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

func calcSignature(encodedHeader, encodedPayload string) (string, error) {
	content := encodedHeader + "." + encodedPayload
	mac := hmac.New(sha256.New, []byte(AppConfig.JWTSecret))
	_, err := mac.Write([]byte(content))
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
