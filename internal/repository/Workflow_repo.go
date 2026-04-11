package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WorkflowRepository interface {

	//Devices
	CreateDevice(d *models.WorkflowDevice) error
	UpdateDevice(d *models.WorkflowDevice) error
	DeleteDevice(id uint) error
	GetDevice(id uint) (*models.WorkflowDevice, error)
	ListDevices() ([]models.WorkflowDevice, error)

	//Jobs
	CreateJob(j *models.WorkflowJob) error
	UpdateJob(j *models.WorkflowJob) error
	DeleteJob(id uint) error
	GetJob(id uint) (*models.WorkflowJob, error)
	ListJobs() ([]models.WorkflowJob, error)
	ListEnabledJobs() ([]models.WorkflowJob, error)
	UpdateJobStatus(id uint, status, message string, t time.Time) error

	//Runs
	CreateRun(r *models.WorkflowRun) error
	FinishRun(id uint, status, output, errMsg string, t time.Time) error
	GetRunByID(id uint) (*models.WorkflowRun, error)
	ListRunsForJob(jobID uint, limit int) ([]models.WorkflowRun, error)

	//Logs
	WriteLog(entry *models.WorkflowLog) error
	ListLogs(filter LogFilter) ([]models.WorkflowLog, int64, error)
	DeleteLogsOlderThan(cutoff time.Time) (int64, error)
}

type LogFilter struct {
	Level   string // "" | "info" | "success" | "warning" | "error"
	JobType string // "" | "backup" | "command" | "system"
	Event   string // "" | specific event name
	Search  string // free-text search on message / device_name / host
	Page    int
	PerPage int
}

type workflowRepository struct {
	db    *gorm.DB
	scope string
}

func NewWorkflowRepository(db *gorm.DB) WorkflowRepository {
	return NewWorkflowRepositoryForScope(db, "ip")
}

func NewWorkflowRepositoryForScope(db *gorm.DB, scope string) WorkflowRepository {
	if scope == "" {
		scope = "ip"
	}
	return &workflowRepository{db: db, scope: scope}
}

func (r *workflowRepository) scoped(db *gorm.DB) *gorm.DB {
	if r.scope == "ip" {
		return db.Where("(scope = ? OR scope = '')", r.scope)
	}
	return db.Where("scope = ?", r.scope)
}

// ------------- Devices ---------------

func (r *workflowRepository) CreateDevice(d *models.WorkflowDevice) error {
	d.Scope = r.scope
	return r.db.Create(d).Error
}
func (r *workflowRepository) UpdateDevice(d *models.WorkflowDevice) error {
	d.Scope = r.scope
	return r.db.Save(d).Error
}
func (r *workflowRepository) DeleteDevice(id uint) error {
	return r.scoped(r.db).Delete(&models.WorkflowDevice{}, id).Error
}
func (r *workflowRepository) GetDevice(id uint) (*models.WorkflowDevice, error) {
	var device models.WorkflowDevice
	return &device, r.scoped(r.db).First(&device, id).Error
}
func (r *workflowRepository) ListDevices() ([]models.WorkflowDevice, error) {
	var out []models.WorkflowDevice
	return out, r.scoped(r.db).Order("name").Find(&out).Error
}

// -------------- Jobs ----------------

func (r *workflowRepository) CreateJob(j *models.WorkflowJob) error {
	j.Scope = r.scope
	return r.db.Create(j).Error
}
func (r *workflowRepository) UpdateJob(j *models.WorkflowJob) error {
	j.Scope = r.scope
	return r.db.Save(j).Error
}
func (r *workflowRepository) DeleteJob(id uint) error {
	return r.scoped(r.db).Select(clause.Associations).Delete(&models.WorkflowJob{}, id).Error
}

func (r *workflowRepository) GetJob(id uint) (*models.WorkflowJob, error) {
	var j models.WorkflowJob
	return &j, r.scoped(r.db.Preload("Device")).First(&j, id).Error
}
func (r *workflowRepository) ListJobs() ([]models.WorkflowJob, error) {
	var out []models.WorkflowJob
	return out, r.scoped(r.db.Preload("Device")).Order("created_at desc").Find(&out).Error
}
func (r *workflowRepository) ListEnabledJobs() ([]models.WorkflowJob, error) {
	var out []models.WorkflowJob
	return out, r.scoped(r.db.Preload("Device")).Where("enabled = true").Find(&out).Error
}
func (r *workflowRepository) UpdateJobStatus(id uint, status, message string, t time.Time) error {
	return r.scoped(r.db.Model(&models.WorkflowJob{})).Where("id = ?", id).Updates(map[string]interface{}{
		"last_status":  status,
		"last_message": message,
		"last_run_at":  t,
	}).Error
}

// --------------- Runs ------------------

func (r *workflowRepository) CreateRun(run *models.WorkflowRun) error {
	run.Scope = r.scope
	return r.db.Create(run).Error
}
func (r *workflowRepository) FinishRun(id uint, status, output, errMsg string, t time.Time) error {
	return r.scoped(r.db.Model(&models.WorkflowRun{})).Where("id = ?", id).Updates(map[string]interface{}{
		"status":      status,
		"output":      output,
		"error_msg":   errMsg,
		"finished_at": t,
	}).Error
}
func (r *workflowRepository) GetRunByID(id uint) (*models.WorkflowRun, error) {
	var run models.WorkflowRun
	return &run, r.scoped(r.db).First(&run, id).Error
}
func (r *workflowRepository) ListRunsForJob(jobID uint, limit int) ([]models.WorkflowRun, error) {
	var out []models.WorkflowRun
	return out, r.scoped(r.db).Where("job_id = ?", jobID).Order("started_at desc").Limit(limit).Find(&out).Error
}

// ----------------- logs -----------------------

func (r *workflowRepository) WriteLog(entry *models.WorkflowLog) error {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	entry.Scope = r.scope
	return r.db.Create(entry).Error
}
func (r *workflowRepository) ListLogs(f LogFilter) ([]models.WorkflowLog, int64, error) {
	q := r.scoped(r.db.Model(&models.WorkflowLog{}))

	if f.Level != "" {
		q = q.Where("level = ?", f.Level)
	}
	if f.JobType != "" {
		q = q.Where("job_type = ?", f.JobType)
	}
	if f.Event != "" {
		q = q.Where("event = ?", f.Event)
	}
	if f.Search != "" {
		pat := "%" + f.Search + "%"
		q = q.Where("message ILIKE ? OR device_name ILIKE ? OR host ILIKE ? OR command ILIKE ?",
			pat, pat, pat, pat)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	perPage := f.PerPage
	if perPage <= 0 {
		perPage = 50
	}
	page := f.Page
	if page <= 0 {
		page = 1
	}

	var out []models.WorkflowLog
	err := q.Order("created_at desc").Offset((page - 1) * perPage).Limit(perPage).Find(&out).Error

	return out, total, err
}

func (r *workflowRepository) DeleteLogsOlderThan(cutoff time.Time) (int64, error) {
	result := r.scoped(r.db).Where("created_at < ?", cutoff).Delete(&models.WorkflowLog{})
	return result.RowsAffected, result.Error
}
