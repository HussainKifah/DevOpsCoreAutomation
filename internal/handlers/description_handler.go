package handlers

import (
	"net/http"

	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type DescriptionHandler struct {
	Repo repository.DescriptionRepository
}

func NewDescriptionHandler(r repository.DescriptionRepository) *DescriptionHandler {
	return &DescriptionHandler{Repo: r}
}

func (h *DescriptionHandler) GetAll(c *gin.Context) {
	data, err := h.Repo.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *DescriptionHandler) GetByHost(c *gin.Context) {
	host := c.Param("host")
	data, err := h.Repo.GetByHost(host)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}
