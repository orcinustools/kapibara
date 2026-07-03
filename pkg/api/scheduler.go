package api

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/orcinustools/kapibara/pkg/store"
)

// StartScheduler runs a background loop that fires due scheduled backups. It
// ticks once a minute and returns when ctx is cancelled.
func (s *Server) StartScheduler(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runDueBackups(ctx)
		}
	}
}

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// backupDue reports whether a scheduled backup should run now, based on its
// cron expression and last run time.
func backupDue(b *store.Backup) bool {
	if b.Cron == "" {
		return false
	}
	sched, err := cronParser.Parse(b.Cron)
	if err != nil {
		return false
	}
	from := b.CreatedAt
	if b.LastRunAt != nil {
		from = *b.LastRunAt
	}
	next := sched.Next(from)
	return !next.IsZero() && !next.After(time.Now())
}
