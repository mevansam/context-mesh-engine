// Use of this source code is governed by the Apache 2.0 license
// that can be found in the LICENSE file.

package jwtx

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	TokenUseClient = "client"
	TokenUseUser   = "user"
	AudienceMCP    = "petstore-mcp"
	IssuerAuth     = "petstore-auth"
	IssuerEngine   = "petstore-mcp"
)

// ClientClaims is the calling-application bearer token.
type ClientClaims struct {
	TokenUse string `json:"token_use"`
	jwt.RegisteredClaims
}

// UserClaims is the end-user JWT (X-End-User-Token).
type UserClaims struct {
	TokenUse   string `json:"token_use"`
	Username   string `json:"username"`
	UserStatus int    `json:"userStatus"`
	jwt.RegisteredClaims
}

// DownstreamClaims is minted by mcp-server for Petstore HTTP.
type DownstreamClaims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func SignClient(secret []byte, clientID string, ttl time.Duration) (string, error) {
	now := time.Now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, ClientClaims{
		TokenUse: TokenUseClient,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    IssuerAuth,
			Subject:   clientID,
			Audience:  jwt.ClaimStrings{AudienceMCP},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	})
	return t.SignedString(secret)
}

func SignUser(secret []byte, username string, userStatus int, ttl time.Duration) (string, error) {
	now := time.Now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, UserClaims{
		TokenUse:   TokenUseUser,
		Username:   username,
		UserStatus: userStatus,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    IssuerAuth,
			Subject:   username,
			Audience:  jwt.ClaimStrings{AudienceMCP},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	})
	return t.SignedString(secret)
}

func SignDownstream(secret []byte, username string, ttl time.Duration) (string, error) {
	now := time.Now()
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, DownstreamClaims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    IssuerEngine,
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	})
	return t.SignedString(secret)
}

func ParseClient(secret []byte, token string) (*ClientClaims, error) {
	claims := &ClientClaims{}
	if err := parseHS256(secret, token, claims); err != nil {
		return nil, err
	}
	if claims.TokenUse != TokenUseClient {
		return nil, fmt.Errorf("token_use %q is not client", claims.TokenUse)
	}
	return claims, nil
}

func ParseUser(secret []byte, token string) (*UserClaims, error) {
	claims := &UserClaims{}
	if err := parseHS256(secret, token, claims); err != nil {
		return nil, err
	}
	if claims.TokenUse != TokenUseUser {
		return nil, fmt.Errorf("token_use %q is not user", claims.TokenUse)
	}
	if claims.Username == "" {
		return nil, fmt.Errorf("username claim missing")
	}
	return claims, nil
}

func parseHS256(secret []byte, token string, claims jwt.Claims) error {
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	return err
}
