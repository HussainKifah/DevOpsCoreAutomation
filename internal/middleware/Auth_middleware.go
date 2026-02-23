package middleware

import (
	"fmt"
	"net/http"
	"strings"

	auth "github.com/Flafl/DevOpsCore/internal/Auth"
	"github.com/gin-gonic/gin"
)

func AuthMiddleware(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authHeader := ctx.GetHeader("Authorization")
		if authHeader == "" {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header required",
			})
			ctx.Abort()
			return
		}
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid Authorization header format",
			})
			ctx.Abort()
			return
		}
		token := tokenParts[1]
		claims, err := jwtManager.ValidateToken(token)
		if err != nil {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid or expired token",
			})
			ctx.Abort()
			return
		}
		if claims.TokenType != "access" {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid token type",
			})
			ctx.Abort()
			return
		}
		ctx.Set("user", claims)
		ctx.Next()
	}
}

func RequirePermission(rbac *auth.RBAC, resource, action string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userInterface, exists := ctx.Get("user")
		if !exists {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error": "User not authenticated",
			})
			ctx.Abort()
			return
		}
		claims, ok := userInterface.(*auth.Claims)
		if !ok {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": "Invalid user data",
			})
			ctx.Abort()
			return
		}
		if !rbac.HasPermission(claims.UserID, resource, action) {
			ctx.JSON(http.StatusForbidden, gin.H{
				"error":    "Insufficient permissions",
				"required": fmt.Sprintf("%s:%s", resource, action),
			})
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

func RequireRole(rbac *auth.RBAC, roleName string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		userInterface, exists := ctx.Get("user")
		if !exists {
			ctx.JSON(http.StatusUnauthorized, gin.H{
				"error": "User not authenticated",
			})
			ctx.Abort()
			return
		}
		clamis, ok := userInterface.(*auth.Claims)
		if !ok {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": "Invalid user data",
			})
			ctx.Abort()
			return
		}
		if !rbac.HasRole(clamis.UserID, roleName) {
			ctx.JSON(http.StatusForbidden, gin.H{
				"error":    "Insufficient role",
				"required": roleName,
			})
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}
