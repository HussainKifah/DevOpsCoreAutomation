package handlers

import (
	"net/http"
	"strconv"

	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type UserhHandler struct {
	userRepo repository.UserRepository
}

func NewUserHandler(userRepo repository.UserRepository) *UserhHandler {
	return &UserhHandler{
		userRepo: userRepo,
	}
}

func (h *UserhHandler) Create(c *gin.Context) {
	var req struct {
		FullName string `json:"full_name" binding:"required min=2, max=100"`
		Email    string `json:"email" binding:"required email"`
		Password string `json:"password" binding:"required, min=8"`
		Role     string `json:"role" binding:"required, oneof=user admin"`
	}

	if err := c.ShouldBindBodyWithJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid input",
			"details": err.Error(),
		})
		return
	}

	if existingUser, _ := h.userRepo.GetByEmail(req.Email); existingUser != nil {
		c.JSON(http.StatusConflict, gin.H{
			"error": "Email already exists",
		})
		return
	}

	user := &models.User{
		Fullname: req.FullName,
		Email:    req.Email,
		Password: req.Password,
		Role:     req.Role,
	}

	if err := h.userRepo.Create(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create user",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "User created successfully",
		"user":    user,
	})
}

func (h *UserhHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	search := c.Query("search")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit

	var users []models.User
	var total int64
	var err error

	if search != "" {
		users, total, err = h.userRepo.Search(search, offset, limit)
	} else {
		users, total, err = h.userRepo.List(offset, limit)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve users",
		})
		return
	}

	totalPages := (total + int64(limit) - 1) / int64(limit)

	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
			"has_next":    page < int(totalPages),
			"has_prev":    page > 1,
		},
	})
}

func (h *UserhHandler) UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}
	user, err := h.userRepo.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"errors": err.Error(),
		})
		return
	}
	var req struct {
		FullName string `json:"full_name" binding:"omitempty,min=2,max=100"`
		Email    string `json:"email" binding:"omitempty,email"`
		Role     string `json:"role" binding:"omitempty,oneof=user admin"`
		Active   *bool  `json:"active"`
	}

	if err := c.ShouldBindBodyWithJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid input",
			"details": err.Error(),
		})
		return
	}

	if req.FullName != "" {
		user.Fullname = req.FullName
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Role != "" {
		user.Role = req.Role
	}
	if req.Active != nil {
		user.Active = *req.Active
	}

	if err := h.userRepo.Update(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to update user",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "User updated successfully",
		"user":    user,
	})
}

func (h *UserhHandler) DeleteUser(c *gin.Context) {

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid user ID",
		})
		return
	}

	if err := h.userRepo.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to delete user",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "User deleted successfully",
	})
}
