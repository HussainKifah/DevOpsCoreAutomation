package handlers

import (
	"net/http"
	"strings"
	"time"

	auth "github.com/Flafl/DevOpsCore/internal/Auth"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	userRepo       repository.UserRepository
	jwtManager     *auth.JWTManager
	passwordHasher *auth.PasswordHasher
}

func NewAuthHandler(userRepo repository.UserRepository, jwtManager *auth.JWTManager) *AuthHandler {
	return &AuthHandler{
		userRepo:       userRepo,
		jwtManager:     jwtManager,
		passwordHasher: auth.NewPasswordHasher(),
	}
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
	Remember bool   `json:"remember"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindBodyWithJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid input",
			"details": err.Error(),
		})
		return
	}
	var user *models.User
	var err error

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	if !h.passwordHasher.CheckPassword(req.Password, user.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid credentials",
		})
		return
	}

	if !user.Active {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Account is disabled",
		})
		return
	}

	tokenPair, err := h.jwtManager.GenerateTokenPair(user.ID, user.Email, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate tokens",
		})
		return
	}

	if req.Remember {
		c.SetCookie(
			"refresh_token",
			tokenPair.RefreshToken,
			int(time.Hour.Seconds()*24*30),
			"/",
			"",
			true,
			true,
		)
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Login successful",
		"user": gin.H{
			"id":        user.ID,
			"email":     user.Email,
			"full_name": user.Fullname,
			"role":      user.Role,
		},
		"tokens": tokenPair,
	})
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Refresh token required",
			})
			return
		}
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid token format",
			})
			return
		}
		refreshToken = tokenParts[1]
	}

	tokenPair, err := h.jwtManager.RefreshAccessToken(refreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid refresh token",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Token refreshed successfully",
		"tokens":  tokenPair,
	})
}
func (h *AuthHandler) Logout(c *gin.Context) {
	c.SetCookie(
		"refresh_token",
		"",
		-1,
		"/",
		"",
		true,
		true,
	)
	c.JSON(http.StatusOK, gin.H{
		"message": "Logged out successfully",
	})
}

func (h *AuthHandler) GetProfile(c *gin.Context) {
	userInterface, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "User not found in context",
		})
		return
	}

	claims, ok := userInterface.(*auth.Claims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Invalid user data",
		})
		return
	}
	user, err := h.userRepo.GetByID(claims.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "User not found",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":         user.ID,
			"email":      user.Email,
			"full_name":  user.Fullname,
			"role":       user.Role,
			"created_at": user.CreatedAt,
		},
	})
}
