// Package jwt provides JWT token generation and validation.
package jwt

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/infrastructure/config"
)

// Custom errors.
var (
	ErrInvalidToken     = errors.New("invalid token")
	ErrExpiredToken     = errors.New("token has expired")
	ErrInvalidTokenType = errors.New("invalid token type")
)

// TokenType represents the type of JWT token.
type TokenType string

// Token type constants.
const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// Claims represents the JWT claims for IAM service.
type Claims struct {
	jwt.RegisteredClaims
	TokenType     TokenType `json:"token_type"`
	UserID        string    `json:"user_id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	Roles         []string  `json:"roles,omitempty"`
	Permissions   []string  `json:"permissions,omitempty"`
	ServiceAccess []string  `json:"service_access,omitempty"`
}

// Service provides JWT token operations.
type Service struct {
	accessSecret  []byte
	refreshSecret []byte
	accessTTL     time.Duration
	refreshTTL    time.Duration
	issuer        string
}

// NewService creates a new JWT service.
func NewService(cfg *config.JWTConfig) *Service {
	return &Service{
		accessSecret:  []byte(cfg.AccessTokenSecret),
		refreshSecret: []byte(cfg.RefreshTokenSecret),
		accessTTL:     cfg.AccessTokenTTL,
		refreshTTL:    cfg.RefreshTokenTTL,
		issuer:        cfg.Issuer,
	}
}

// TokenPair represents an access and refresh token pair.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	AccessExp    time.Time
	RefreshExp   time.Time
	TokenID      string // JTI for the refresh token (used for revocation)
}

// GenerateTokenPair generates a new access and refresh token pair.
func (s *Service) GenerateTokenPair(userID uuid.UUID, username, email string, roles, permissions, serviceAccess []string) (*TokenPair, error) {
	now := time.Now()
	tokenID := uuid.New().String()

	// Generate access token
	accessExp := now.Add(s.accessTTL)
	accessClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExp),
			ID:        uuid.New().String(),
		},
		TokenType:     TokenTypeAccess,
		UserID:        userID.String(),
		Username:      username,
		Email:         email,
		Roles:         roles,
		Permissions:   permissions,
		ServiceAccess: serviceAccess,
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(s.accessSecret)
	if err != nil {
		return nil, err
	}

	// Generate refresh token
	refreshExp := now.Add(s.refreshTTL)
	refreshClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(refreshExp),
			ID:        tokenID,
		},
		TokenType: TokenTypeRefresh,
		UserID:    userID.String(),
		Username:  username,
		Email:     email,
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString(s.refreshSecret)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		AccessExp:    accessExp,
		RefreshExp:   refreshExp,
		TokenID:      tokenID,
	}, nil
}

// ValidateAccessToken validates an access token and returns the claims.
func (s *Service) ValidateAccessToken(tokenString string) (*Claims, error) {
	return s.validateToken(tokenString, TokenTypeAccess, s.accessSecret)
}

// ValidateRefreshToken validates a refresh token and returns the claims.
func (s *Service) ValidateRefreshToken(tokenString string) (*Claims, error) {
	return s.validateToken(tokenString, TokenTypeRefresh, s.refreshSecret)
}

func (s *Service) validateToken(tokenString string, expectedType TokenType, secret []byte) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return secret, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	if claims.TokenType != expectedType {
		return nil, ErrInvalidTokenType
	}

	return claims, nil
}

// GetAccessTTLSeconds returns the access token TTL in seconds.
func (s *Service) GetAccessTTLSeconds() int64 {
	return int64(s.accessTTL.Seconds())
}

// GetRefreshTTLSeconds returns the refresh token TTL in seconds.
func (s *Service) GetRefreshTTLSeconds() int64 {
	return int64(s.refreshTTL.Seconds())
}
