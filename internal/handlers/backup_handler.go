package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type BackupHandler struct {
	Repo repository.BackupRepository
}

func NewBackupHandler(r repository.BackupRepository) *BackupHandler {
	return &BackupHandler{Repo: r}
}

func (h *BackupHandler) GetAll(c *gin.Context) {
	if site := c.Query("site"); site != "" {
		data, err := h.Repo.GetBySite(site)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, data)
		return
	}

	data, err := h.Repo.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *BackupHandler) Download(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	backup, err := h.Repo.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}

	if _, err := os.Stat(backup.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found on disk"})
		return
	}

	c.FileAttachment(backup.FilePath, filepath.Base(backup.FilePath))
}
