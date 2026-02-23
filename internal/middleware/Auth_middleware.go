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
		var token string

		if cookie, err := ctx.Cookie("access_token"); err == nil && cookie != "" {
			token = cookie
		}

		if token == "" {
			authHeader := ctx.GetHeader("Authorization")
			if authHeader != "" {
				tokenParts := strings.Split(authHeader, " ")
				if len(tokenParts) == 2 && tokenParts[0] == "Bearer" {
					token = tokenParts[1]
				}
			}
		}

		if token == "" {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
			ctx.Abort()
			return
		}

		claims, err := jwtManager.ValidateToken(token)
		if err != nil {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			ctx.Abort()
			return
		}
		if claims.TokenType != "access" {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token type"})
			ctx.Abort()
			return
		}
		ctx.Set("user", claims)
		ctx.Next()
	}
}

func PageAuthMiddleware(jwtManager *auth.JWTManager) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var token string

		if cookie, err := ctx.Cookie("access_token"); err == nil && cookie != "" {
			token = cookie
		}

		if token == "" {
			ctx.Redirect(http.StatusFound, "/login")
			ctx.Abort()
			return
		}

		claims, err := jwtManager.ValidateToken(token)
		if err != nil {
			ctx.SetCookie("access_token", "", -1, "/", "", false, true)
			ctx.SetCookie("refresh_token", "", -1, "/", "", false, true)
			ctx.Redirect(http.StatusFound, "/login")
			ctx.Abort()
			return
		}
		if claims.TokenType != "access" {
			ctx.Redirect(http.StatusFound, "/login")
			ctx.Abort()
			return
		}
		ctx.Set("user", claims)
		ctx.Next()
	}
}

// RoleGuard checks the role from JWT claims directly (for API routes).
// Returns 403 JSON if the user's role is not in the allowed list.
func RoleGuard(allowedRoles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(allowedRoles))
	for _, r := range allowedRoles {
		allowed[r] = true
	}
	return func(ctx *gin.Context) {
		userInterface, exists := ctx.Get("user")
		if !exists {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Not authenticated"})
			ctx.Abort()
			return
		}
		claims, ok := userInterface.(*auth.Claims)
		if !ok {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid user data"})
			ctx.Abort()
			return
		}
		if !allowed[claims.Role] {
			ctx.JSON(http.StatusForbidden, gin.H{
				"error":    "Insufficient role",
				"your_role": claims.Role,
				"required": fmt.Sprintf("one of %v", allowedRoles),
			})
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}

// PageRoleGuard checks the role from JWT claims (for page routes).
// Redirects to /dashboard if the user's role is not allowed.
func PageRoleGuard(allowedRoles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(allowedRoles))
	for _, r := range allowedRoles {
		allowed[r] = true
	}
	return func(ctx *gin.Context) {
		userInterface, exists := ctx.Get("user")
		if !exists {
			ctx.Redirect(http.StatusFound, "/login")
			ctx.Abort()
			return
		}
		claims, ok := userInterface.(*auth.Claims)
		if !ok {
			ctx.Redirect(http.StatusFound, "/login")
			ctx.Abort()
			return
		}
		if !allowed[claims.Role] {
			ctx.Redirect(http.StatusFound, "/dashboard")
			ctx.Abort()
			return
		}
		ctx.Next()
	}
}
