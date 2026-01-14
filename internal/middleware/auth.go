package middleware

import (
	"errors"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/dollarkillerx/MessageBoy/internal/conf"
	"github.com/dollarkillerx/MessageBoy/pkg/common/resp"
)

type JWTManager struct {
	secretKey []byte
	issuer    string
	expireHrs int
}

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func NewJWTManager(cfg *conf.JWTConfig) *JWTManager {
	return &JWTManager{
		secretKey: []byte(cfg.SecretKey),
		issuer:    cfg.Issuer,
		expireHrs: cfg.ExpireHours,
	}
}

func (m *JWTManager) GenerateToken(username string) (string, time.Time, error) {
	expireAt := time.Now().Add(time.Duration(m.expireHrs) * time.Hour)

	claims := Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    m.issuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(m.secretKey)
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expireAt, nil
}

func (m *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secretKey, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

const (
	AuthHeaderKey = "Authorization"
	BearerPrefix  = "Bearer "
	ContextUser   = "user"
)

func (m *JWTManager) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader(AuthHeaderKey)
		if authHeader == "" {
			resp.ErrorResponse(c, "", resp.ErrCodeAuthRequired, "authorization header required")
			c.Abort()
			return
		}

		if !strings.HasPrefix(authHeader, BearerPrefix) {
			resp.ErrorResponse(c, "", resp.ErrCodeAuthRequired, "invalid authorization format")
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, BearerPrefix)
		claims, err := m.ValidateToken(tokenString)
		if err != nil {
			resp.ErrorResponse(c, "", resp.ErrCodeAuthRequired, "invalid or expired token")
			c.Abort()
			return
		}

		c.Set(ContextUser, claims.Username)
		c.Next()
	}
}

func GetCurrentUser(c *gin.Context) string {
	if user, exists := c.Get(ContextUser); exists {
		return user.(string)
	}
	return ""
}
