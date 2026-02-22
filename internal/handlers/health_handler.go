package handlers

import (
	"net/http"

	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type HealthHandler struct {
	Repo *repository.HealthRepo
}

func NewHealthHandler(r *repository.HealthRepo) *HealthHandler {
	return &HealthHandler{Repo: r}
}

func (h *HealthHandler) GetAll(c *gin.Context) {
	data, err := h.Repo.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *HealthHandler) GetByHost(c *gin.Context) {
	host := c.Param("host")
	data, err := h.Repo.GetByHost(host)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}
