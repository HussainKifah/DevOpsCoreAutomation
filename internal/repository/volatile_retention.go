package repository

import (
	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

// PurgeSoftDeletedVolatileRows permanently removes rows that were soft-deleted (deleted_at set)
// from live OLT snapshot tables. The scheduler calls this after each power, description, port,
// and health job. It does not touch health_snapshots, port_snapshots, backups, or inventory history.
func PurgeSoftDeletedVolatileRows(db *gorm.DB) (powerN, descN, portN, healthN int64, err error) {
	r := db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&models.PowerReading{})
	if r.Error != nil {
		return 0, 0, 0, 0, r.Error
	}
	powerN = r.RowsAffected

	r = db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&models.OntDescription{})
	if r.Error != nil {
		return powerN, 0, 0, 0, r.Error
	}
	descN = r.RowsAffected

	r = db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&models.PortProtectionRecord{})
	if r.Error != nil {
		return powerN, descN, 0, 0, r.Error
	}
	portN = r.RowsAffected

	r = db.Unscoped().Where("deleted_at IS NOT NULL").Delete(&models.OltHealth{})
	if r.Error != nil {
		return powerN, descN, portN, 0, r.Error
	}
	healthN = r.RowsAffected

	return powerN, descN, portN, healthN, nil
}
