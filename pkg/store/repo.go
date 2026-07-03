package store

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrNotFound is returned when a lookup finds no matching row.
var ErrNotFound = errors.New("not found")

func wrap(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// --- users ---

// CreateUser inserts a user.
func (s *Store) CreateUser(u *User) error { return s.DB.Create(u).Error }

// UpdateUser saves changes to a user.
func (s *Store) UpdateUser(u *User) error { return s.DB.Save(u).Error }

// CountUsers returns the number of users (used to bootstrap the first admin).
func (s *Store) CountUsers() (int64, error) {
	var n int64
	err := s.DB.Model(&User{}).Count(&n).Error
	return n, err
}

// UserByEmail looks up a user by email.
func (s *Store) UserByEmail(email string) (*User, error) {
	var u User
	if err := s.DB.Where("email = ?", email).First(&u).Error; err != nil {
		return nil, wrap(err)
	}
	return &u, nil
}

// UserByID looks up a user by id.
func (s *Store) UserByID(id string) (*User, error) {
	var u User
	if err := s.DB.First(&u, "id = ?", id).Error; err != nil {
		return nil, wrap(err)
	}
	return &u, nil
}

// --- organizations & memberships ---

// CreateOrg inserts an organization.
func (s *Store) CreateOrg(o *Organization) error { return s.DB.Create(o).Error }

// CreateMembership links a user to an org with a role.
func (s *Store) CreateMembership(m *Membership) error { return s.DB.Create(m).Error }

// OrgsForUser returns the organizations a user belongs to.
func (s *Store) OrgsForUser(userID string) ([]Organization, error) {
	var orgs []Organization
	err := s.DB.
		Joins("JOIN memberships ON memberships.organization_id = organizations.id").
		Where("memberships.user_id = ? AND memberships.deleted_at IS NULL", userID).
		Find(&orgs).Error
	return orgs, err
}

// Membership returns a user's membership in an org, or ErrNotFound.
func (s *Store) Membership(userID, orgID string) (*Membership, error) {
	var m Membership
	err := s.DB.Where("user_id = ? AND organization_id = ?", userID, orgID).First(&m).Error
	if err != nil {
		return nil, wrap(err)
	}
	return &m, nil
}

// --- projects ---

// CreateProject inserts a project.
func (s *Store) CreateProject(p *Project) error { return s.DB.Create(p).Error }

// ProjectsForOrg lists projects in an organization.
func (s *Store) ProjectsForOrg(orgID string) ([]Project, error) {
	var ps []Project
	err := s.DB.Where("organization_id = ?", orgID).Order("created_at DESC").Find(&ps).Error
	return ps, err
}

// ProjectByID looks up a project.
func (s *Store) ProjectByID(id string) (*Project, error) {
	var p Project
	if err := s.DB.First(&p, "id = ?", id).Error; err != nil {
		return nil, wrap(err)
	}
	return &p, nil
}

// DeleteProject soft-deletes a project row.
func (s *Store) DeleteProject(id string) error {
	return s.DB.Delete(&Project{}, "id = ?", id).Error
}

// --- compose apps ---

// CreateComposeApp inserts a compose app.
func (s *Store) CreateComposeApp(c *ComposeApp) error { return s.DB.Create(c).Error }

// UpdateComposeApp saves changes to a compose app.
func (s *Store) UpdateComposeApp(c *ComposeApp) error { return s.DB.Save(c).Error }

// ComposeAppsForProject lists a project's compose apps.
func (s *Store) ComposeAppsForProject(projectID string) ([]ComposeApp, error) {
	var cs []ComposeApp
	err := s.DB.Where("project_id = ?", projectID).Order("created_at").Find(&cs).Error
	return cs, err
}

// ComposeAppByID looks up a compose app.
func (s *Store) ComposeAppByID(id string) (*ComposeApp, error) {
	var c ComposeApp
	if err := s.DB.First(&c, "id = ?", id).Error; err != nil {
		return nil, wrap(err)
	}
	return &c, nil
}

// --- applications ---

// CreateApplication inserts an application.
func (s *Store) CreateApplication(a *Application) error { return s.DB.Create(a).Error }

// UpdateApplication saves changes to an application.
func (s *Store) UpdateApplication(a *Application) error { return s.DB.Save(a).Error }

// ApplicationsForProject lists a project's applications.
func (s *Store) ApplicationsForProject(projectID string) ([]Application, error) {
	var as []Application
	err := s.DB.Where("project_id = ?", projectID).Order("created_at").Find(&as).Error
	return as, err
}

// ApplicationByID looks up an application.
func (s *Store) ApplicationByID(id string) (*Application, error) {
	var a Application
	if err := s.DB.First(&a, "id = ?", id).Error; err != nil {
		return nil, wrap(err)
	}
	return &a, nil
}

// --- databases ---

// CreateDatabase inserts a database.
func (s *Store) CreateDatabase(d *Database) error { return s.DB.Create(d).Error }

// UpdateDatabase saves changes to a database.
func (s *Store) UpdateDatabase(d *Database) error { return s.DB.Save(d).Error }

// DatabasesForProject lists a project's databases.
func (s *Store) DatabasesForProject(projectID string) ([]Database, error) {
	var ds []Database
	err := s.DB.Where("project_id = ?", projectID).Order("created_at").Find(&ds).Error
	return ds, err
}

// DatabaseByID looks up a database.
func (s *Store) DatabaseByID(id string) (*Database, error) {
	var d Database
	if err := s.DB.First(&d, "id = ?", id).Error; err != nil {
		return nil, wrap(err)
	}
	return &d, nil
}

// ApplicationByWebhookSecret resolves an application by its webhook secret.
func (s *Store) ApplicationByWebhookSecret(secret string) (*Application, error) {
	var a Application
	if err := s.DB.First(&a, "webhook_secret = ?", secret).Error; err != nil {
		return nil, wrap(err)
	}
	return &a, nil
}

// DeploymentByID looks up a deployment.
func (s *Store) DeploymentByID(id string) (*Deployment, error) {
	var d Deployment
	if err := s.DB.First(&d, "id = ?", id).Error; err != nil {
		return nil, wrap(err)
	}
	return &d, nil
}

// --- deployments ---

// CreateDeployment inserts a deployment record.
func (s *Store) CreateDeployment(d *Deployment) error { return s.DB.Create(d).Error }

// UpdateDeployment saves changes to a deployment record.
func (s *Store) UpdateDeployment(d *Deployment) error { return s.DB.Save(d).Error }

// DeploymentsForProject lists a project's deployments, newest first.
func (s *Store) DeploymentsForProject(projectID string) ([]Deployment, error) {
	var ds []Deployment
	err := s.DB.Where("project_id = ?", projectID).Order("created_at DESC").Limit(100).Find(&ds).Error
	return ds, err
}

// --- notifications ---

// CreateNotification inserts a notification channel.
func (s *Store) CreateNotification(n *Notification) error { return s.DB.Create(n).Error }

// NotificationsForOrg lists an org's notification channels.
func (s *Store) NotificationsForOrg(orgID string) ([]Notification, error) {
	var ns []Notification
	err := s.DB.Where("organization_id = ?", orgID).Find(&ns).Error
	return ns, err
}

// DeleteNotification removes a notification channel.
func (s *Store) DeleteNotification(id string) error {
	return s.DB.Delete(&Notification{}, "id = ?", id).Error
}

// --- backups ---

// CreateBackup inserts a backup config.
func (s *Store) CreateBackup(b *Backup) error { return s.DB.Create(b).Error }

// UpdateBackup saves changes to a backup config.
func (s *Store) UpdateBackup(b *Backup) error { return s.DB.Save(b).Error }

// BackupByID looks up a backup config.
func (s *Store) BackupByID(id string) (*Backup, error) {
	var b Backup
	if err := s.DB.First(&b, "id = ?", id).Error; err != nil {
		return nil, wrap(err)
	}
	return &b, nil
}

// BackupsForProject lists a project's backup configs.
func (s *Store) BackupsForProject(projectID string) ([]Backup, error) {
	var bs []Backup
	err := s.DB.Where("project_id = ?", projectID).Find(&bs).Error
	return bs, err
}

// EnabledScheduledBackups lists backups that have a cron and are enabled.
func (s *Store) EnabledScheduledBackups() ([]Backup, error) {
	var bs []Backup
	err := s.DB.Where("enabled = ? AND cron <> ''", true).Find(&bs).Error
	return bs, err
}

// --- audit log ---

// CreateAuditLog records an action.
func (s *Store) CreateAuditLog(a *AuditLog) error { return s.DB.Create(a).Error }

// RecentAuditLogs returns the most recent audit entries.
func (s *Store) RecentAuditLogs(limit int) ([]AuditLog, error) {
	var as []AuditLog
	err := s.DB.Order("created_at DESC").Limit(limit).Find(&as).Error
	return as, err
}

// --- api tokens ---

// CreateAPIToken inserts an API token.
func (s *Store) CreateAPIToken(t *ApiToken) error { return s.DB.Create(t).Error }

// UserByAPITokenHash resolves the user owning an API token by its hash and
// stamps LastUsed.
func (s *Store) UserByAPITokenHash(hash string) (*User, error) {
	var tok ApiToken
	if err := s.DB.Where("token_hash = ?", hash).First(&tok).Error; err != nil {
		return nil, wrap(err)
	}
	now := time.Now()
	s.DB.Model(&tok).Update("last_used", &now)
	return s.UserByID(tok.UserID)
}

// APITokensForUser lists a user's API tokens.
func (s *Store) APITokensForUser(userID string) ([]ApiToken, error) {
	var ts []ApiToken
	err := s.DB.Where("user_id = ?", userID).Order("created_at DESC").Find(&ts).Error
	return ts, err
}
