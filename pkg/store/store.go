// Package store is the kapibara control-plane persistence layer.
//
// It uses GORM over SQLite (pure-Go, no CGO) for local/dev and Postgres for
// production, selected by the database URL. AutoMigrate keeps the schema in
// sync with the models as milestones add entities.
package store

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Store wraps a GORM database handle.
type Store struct {
	DB *gorm.DB
}

// AllModels lists every persisted model, in dependency order, for AutoMigrate.
func AllModels() []any {
	return []any{
		&User{},
		&Organization{},
		&Membership{},
		&GitProvider{},
		&Project{},
		&ComposeApp{},
		&Application{},
		&Database{},
		&Deployment{},
		&Notification{},
		&Backup{},
		&AuditLog{},
		&ApiToken{},
	}
}

// Open connects to the store described by url and runs migrations.
//
//   - "" or a path (e.g. /home/x/.kapibara/kapibara.db) → SQLite
//   - "postgres://..." or "postgresql://..." → Postgres
func Open(url string) (*Store, error) {
	var dialector gorm.Dialector
	switch {
	case strings.HasPrefix(url, "postgres://"), strings.HasPrefix(url, "postgresql://"):
		dialector = postgres.Open(url)
	default:
		if url == "" {
			url = "kapibara.db"
		}
		dialector = sqlite.Open(url)
	}

	gormLogger := logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  logger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	})
	db, err := gorm.Open(dialector, &gorm.Config{Logger: gormLogger})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	s := &Store{DB: db}
	if err := s.Migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

// Migrate runs AutoMigrate for all models.
func (s *Store) Migrate() error {
	if err := s.DB.AutoMigrate(AllModels()...); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error {
	sqlDB, err := s.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
