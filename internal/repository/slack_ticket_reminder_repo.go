package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SlackTicketReminderRepository struct {
	db *gorm.DB
}

func NewSlackTicketReminderRepository(db *gorm.DB) *SlackTicketReminderRepository {
	return &SlackTicketReminderRepository{db: db}
}

func (r *SlackTicketReminderRepository) CreateIfMissing(ticket *models.SlackTicketReminder) (bool, error) {
	tx := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(ticket)
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}

func (r *SlackTicketReminderRepository) GetByMessage(channelID, messageTS string) (*models.SlackTicketReminder, error) {
	var ticket models.SlackTicketReminder
	err := r.db.Where("channel_id = ? AND message_ts = ?", channelID, messageTS).Limit(1).Find(&ticket).Error
	if err != nil {
		return nil, err
	}
	if ticket.ID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &ticket, nil
}

func (r *SlackTicketReminderRepository) ListOpenDue(until time.Time) ([]models.SlackTicketReminder, error) {
	var list []models.SlackTicketReminder
	err := r.db.Where("resolved_at IS NULL AND next_reminder_at <= ?", until).
		Order("next_reminder_at ASC").Limit(50).Find(&list).Error
	return list, err
}

func (r *SlackTicketReminderRepository) UpdateReminderState(
	id uint,
	next time.Time,
	remindedAt time.Time,
	lastReplyTS string,
	lastReplyUserID string,
	lastReplyUserName string,
	lastReplyText string,
) error {
	return r.db.Model(&models.SlackTicketReminder{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"next_reminder_at":    next,
			"last_reminder_at":    remindedAt,
			"last_reply_ts":       lastReplyTS,
			"last_reply_user_id":  lastReplyUserID,
			"last_reply_user_name": lastReplyUserName,
			"last_reply_text":     lastReplyText,
		}).Error
}

func (r *SlackTicketReminderRepository) MarkResolved(id uint, resolvedBy string, at time.Time) error {
	far := at.AddDate(50, 0, 0)
	return r.db.Model(&models.SlackTicketReminder{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"resolved_at":       at,
			"resolved_by":       resolvedBy,
			"next_reminder_at":  far,
		}).Error
}
