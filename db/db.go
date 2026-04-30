package db

import (
	"log"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(cfg *config.Config) *gorm.DB {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to get underlying sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	if err := db.AutoMigrate(
		&models.PowerReading{},
		&models.OntDescription{},
		&models.OltHealth{},
		&models.PortProtectionRecord{},
		&models.OltBackups{},
		&models.User{},
		&models.HealthSnapshot{},
		&models.PortSnapshot{},
		&models.InventorySummary{},
		&models.OltInventory{},
		&models.OntInventoryItem{},
		&models.OntInterface{},
		&models.WorkflowDevice{},
		&models.WorkflowJob{},
		&models.WorkflowRun{},
		&models.WorkflowLog{},
		&models.NocPassDevice{},
		&models.NocPassCredential{},
		&models.NocPassPolicy{},
		&models.NocPassKeepUser{},
		&models.NocPassSavedUser{},
		&models.NocPassExclusion{},
		&models.NocDataDevice{},
		&models.NocDataHistory{},
		&models.NocDataCredential{},
		&models.NocDataExclusion{},
		&models.EsSyslogFilter{},
		&models.EsSyslogAlert{},
		&models.EsSyslogSlackIncident{},
		&models.SlackTicketReminder{},
		&models.RuijieMailAlert{},
		&models.RuijieSlackIncident{},
		&models.IPCapacityNode{},
		&models.IPCapacityAction{},
	); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	if err := db.Exec("ALTER TABLE workflow_jobs ALTER COLUMN device_id DROP NOT NULL").Error; err != nil {
		log.Printf("workflow_jobs migration warning: %v", err)
	}
	if err := db.Exec("ALTER TABLE noc_pass_policies ADD COLUMN IF NOT EXISTS name varchar(128)").Error; err != nil {
		log.Printf("noc_pass_policies add name warning: %v", err)
	}
	if err := db.Exec("UPDATE noc_pass_policies SET name = COALESCE(NULLIF(BTRIM(name), ''), target_label, 'NOC PASS Policy')").Error; err != nil {
		log.Printf("noc_pass_policies name backfill warning: %v", err)
	}
	if err := db.Exec("ALTER TABLE noc_pass_keep_users ADD COLUMN IF NOT EXISTS canonical_username varchar(64)").Error; err != nil {
		log.Printf("noc_pass_keep_users add canonical_username warning: %v", err)
	}
	if err := db.Exec("ALTER TABLE noc_pass_policies ADD COLUMN IF NOT EXISTS enc_manual_fiberx_password bytea").Error; err != nil {
		log.Printf("noc_pass_policies add enc_manual_fiberx_password warning: %v", err)
	}
	if err := db.Exec("ALTER TABLE noc_pass_policies ADD COLUMN IF NOT EXISTS enc_manual_support_password bytea").Error; err != nil {
		log.Printf("noc_pass_policies add enc_manual_support_password warning: %v", err)
	}
	if err := db.Exec("ALTER TABLE noc_pass_policies ADD COLUMN IF NOT EXISTS enc_active_fiberx_password bytea").Error; err != nil {
		log.Printf("noc_pass_policies add enc_active_fiberx_password warning: %v", err)
	}
	if err := db.Exec("ALTER TABLE noc_pass_policies ADD COLUMN IF NOT EXISTS enc_active_support_password bytea").Error; err != nil {
		log.Printf("noc_pass_policies add enc_active_support_password warning: %v", err)
	}
	if err := db.Exec("UPDATE noc_pass_policies SET enc_manual_fiberx_password = enc_manual_password WHERE enc_manual_fiberx_password IS NULL AND enc_manual_password IS NOT NULL").Error; err != nil {
		log.Printf("noc_pass_policies backfill enc_manual_fiberx_password warning: %v", err)
	}
	if err := db.Exec("UPDATE noc_pass_policies SET enc_manual_support_password = enc_manual_password WHERE enc_manual_support_password IS NULL AND enc_manual_password IS NOT NULL").Error; err != nil {
		log.Printf("noc_pass_policies backfill enc_manual_support_password warning: %v", err)
	}
	if err := db.Exec("UPDATE noc_pass_policies SET enc_active_fiberx_password = enc_active_password WHERE enc_active_fiberx_password IS NULL AND enc_active_password IS NOT NULL").Error; err != nil {
		log.Printf("noc_pass_policies backfill enc_active_fiberx_password warning: %v", err)
	}
	if err := db.Exec("UPDATE noc_pass_policies SET enc_active_support_password = enc_active_password WHERE enc_active_support_password IS NULL AND enc_active_password IS NOT NULL").Error; err != nil {
		log.Printf("noc_pass_policies backfill enc_active_support_password warning: %v", err)
	}
	if err := db.Exec("UPDATE noc_pass_keep_users SET canonical_username = LOWER(TRIM(username)) WHERE canonical_username IS NULL OR canonical_username = ''").Error; err != nil {
		log.Printf("noc_pass_keep_users canonical backfill warning: %v", err)
	}
	if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_noc_pass_keep_users_canonical_username ON noc_pass_keep_users (canonical_username)").Error; err != nil {
		log.Printf("noc_pass_keep_users canonical index warning: %v", err)
	}
	if err := db.Exec("ALTER TABLE noc_pass_saved_users ADD COLUMN IF NOT EXISTS privilege varchar(16)").Error; err != nil {
		log.Printf("noc_pass_saved_users add privilege warning: %v", err)
	}
	if err := db.Exec("UPDATE noc_pass_saved_users SET privilege = 'full' WHERE privilege IS NULL OR BTRIM(privilege) = ''").Error; err != nil {
		log.Printf("noc_pass_saved_users privilege backfill warning: %v", err)
	}
	if err := db.Exec("ALTER TABLE es_syslog_slack_incidents ADD COLUMN IF NOT EXISTS snoozed_at timestamptz").Error; err != nil {
		log.Printf("es_syslog_slack_incidents add snoozed_at warning: %v", err)
	}
	if err := db.Exec("ALTER TABLE es_syslog_slack_incidents ADD COLUMN IF NOT EXISTS snoozed_by varchar(256)").Error; err != nil {
		log.Printf("es_syslog_slack_incidents add snoozed_by warning: %v", err)
	}
	if err := db.Exec("ALTER TABLE ruijie_slack_incidents ADD COLUMN IF NOT EXISTS snoozed_at timestamptz").Error; err != nil {
		log.Printf("ruijie_slack_incidents add snoozed_at warning: %v", err)
	}
	if err := db.Exec("ALTER TABLE ruijie_slack_incidents ADD COLUMN IF NOT EXISTS snoozed_by varchar(256)").Error; err != nil {
		log.Printf("ruijie_slack_incidents add snoozed_by warning: %v", err)
	}
	if err := db.Exec("DROP INDEX IF EXISTS idx_ip_capacity_nodes_name").Error; err != nil {
		log.Printf("ip_capacity_nodes drop old name-only unique index warning: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_ip_capacity_nodes_identity ON ip_capacity_nodes (province, name, type)").Error; err != nil {
		log.Printf("ip_capacity_nodes identity index warning: %v", err)
	}

	go func() {
		indexes := []string{
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_hs_measured ON health_snapshots (measured_at)",
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_hs_host_measured ON health_snapshots (host, measured_at)",
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ps_measured ON port_snapshots (measured_at)",
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ps_host_measured ON port_snapshots (host, measured_at)",
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_es_syslog_slack_incidents_snoozed_at ON es_syslog_slack_incidents (snoozed_at)",
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ruijie_slack_incidents_snoozed_at ON ruijie_slack_incidents (snoozed_at)",
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_noc_data_histories_run_at_host ON noc_data_histories (run_at, host)",
		}
		for _, ddl := range indexes {
			if err := db.Exec(ddl).Error; err != nil {
				log.Printf("index warning: %v", err)
			}
		}
		log.Println("database indexes ensured")
	}()

	log.Println("database connected and migrated")
	return db
}
