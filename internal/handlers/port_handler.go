package handlers

import (
	"net/http"

	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type PortHandler struct {
	Repo repository.PortProtectionRepository
}

func NewPortHandler(r repository.PortProtectionRepository) *PortHandler {
	return &PortHandler{Repo: r}
}

func (h *PortHandler) GetDown(c *gin.Context) {
	data, err := h.Repo.GetDown()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *PortHandler) GetByHost(c *gin.Context) {
	host := c.Param("host")
	data, err := h.Repo.GetByHost(host)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}
