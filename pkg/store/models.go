package store

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Base is embedded in every model: a UUID primary key + timestamps.
type Base struct {
	ID        string         `gorm:"primaryKey;size:36" json:"id"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate assigns a UUID if the caller did not set one.
func (b *Base) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	return nil
}

// Role is a membership role within an organization.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

// User is a control-plane account.
type User struct {
	Base
	Email        string `gorm:"uniqueIndex;size:255;not null" json:"email"`
	Name         string `gorm:"size:255" json:"name"`
	PasswordHash string `gorm:"size:255;not null" json:"-"`
	IsAdmin      bool   `json:"isAdmin"` // platform super-admin

	// TOTP two-factor auth.
	TOTPSecret  string `gorm:"size:128" json:"-"`
	TwoFAEnabled bool  `json:"twoFAEnabled"`
}

// Organization is a multi-tenant boundary owning projects.
type Organization struct {
	Base
	Name string `gorm:"size:255;not null" json:"name"`
	Slug string `gorm:"uniqueIndex;size:255;not null" json:"slug"`
}

// Membership links a user to an organization with a role.
type Membership struct {
	Base
	UserID         string       `gorm:"index;size:36;not null" json:"userId"`
	OrganizationID string       `gorm:"index;size:36;not null" json:"organizationId"`
	Role           Role         `gorm:"size:32;not null" json:"role"`
	User           User         `gorm:"constraint:OnDelete:CASCADE" json:"-"`
	Organization   Organization `gorm:"constraint:OnDelete:CASCADE" json:"-"`
}

// Project groups applications/databases and maps to an orcinus project.
type Project struct {
	Base
	OrganizationID string `gorm:"index;size:36;not null" json:"organizationId"`
	Name           string `gorm:"size:255;not null" json:"name"`
	// OrcinusProject is the project name used against the orcinus engine.
	OrcinusProject string `gorm:"uniqueIndex;size:255;not null" json:"orcinusProject"`
	Description    string `gorm:"type:text" json:"description"`
}

// ComposeApp is a docker-compose source deployed as one project unit. A
// project may hold several (e.g. app + supporting services).
type ComposeApp struct {
	Base
	ProjectID string `gorm:"index;size:36;not null" json:"projectId"`
	Name      string `gorm:"size:255;not null" json:"name"`
	// OrcinusProject isolates this unit's cluster resources (own prune scope).
	OrcinusProject string `gorm:"uniqueIndex;size:255" json:"orcinusProject"`
	// Source is the raw docker-compose (optionally with x-orcinus-* hints).
	Source string `gorm:"type:text" json:"source"`
}

// Application is a deployable service built from Git or a prebuilt image.
type Application struct {
	Base
	ProjectID string `gorm:"index;size:36;not null" json:"projectId"`
	Name      string `gorm:"size:255;not null" json:"name"`
	// OrcinusProject isolates this unit's cluster resources (own prune scope).
	OrcinusProject string `gorm:"uniqueIndex;size:255" json:"orcinusProject"`

	// BuildType: dockerfile | nixpacks | image
	BuildType string `gorm:"size:32;not null;default:dockerfile" json:"buildType"`

	// Git source (for dockerfile/nixpacks builds).
	RepoURL        string `gorm:"size:512" json:"repoUrl"`
	Branch         string `gorm:"size:255" json:"branch"`
	ContextDir     string `gorm:"size:512" json:"contextDir"`
	DockerfilePath string `gorm:"size:512" json:"dockerfilePath"`

	// Prebuilt image (for build type "image").
	Image string `gorm:"size:512" json:"image"`

	// Runtime config.
	Port     int    `json:"port"`     // container port to expose
	Replicas int    `json:"replicas"` // desired replicas (0 → 1)
	Domain   string `gorm:"size:255" json:"domain"`
	TLS      bool   `json:"tls"`
	// Env is a JSON object of environment variables (secret + non-secret).
	Env string `gorm:"type:text" json:"-"`
	// SecretKeys is a JSON array of Env keys to store as cluster Secrets
	// (rendered as x-orcinus-secret so orcinus extracts them into a Secret).
	SecretKeys string `gorm:"type:text" json:"-"`

	// CurrentImage is the last successfully built/published image ref.
	CurrentImage string `gorm:"size:512" json:"currentImage"`

	// Autoscale (HPA) + progressive delivery.
	AutoscaleMin    int    `json:"autoscaleMin"`
	AutoscaleMax    int    `json:"autoscaleMax"`
	AutoscaleCPU    int    `json:"autoscaleCpu"`
	AutoscaleMemory int    `json:"autoscaleMemory"`
	Rollout         string `gorm:"size:32" json:"rollout"`

	// Resource limits/reservations (compose deploy.resources → k8s
	// limits/requests). CPU in cores (e.g. "0.5"); memory like "512M".
	CPULimit       string `gorm:"size:32" json:"cpuLimit"`
	MemoryLimit    string `gorm:"size:32" json:"memoryLimit"`
	CPURequest     string `gorm:"size:32" json:"cpuRequest"`
	MemoryRequest  string `gorm:"size:32" json:"memoryRequest"`
	// Command overrides the image command; JSON array of args.
	Command string `gorm:"type:text" json:"command"`
	// Mounts is a JSON array of {name,path} persistent volume mounts.
	Mounts string `gorm:"type:text" json:"mounts"`
	// VolumeSize is the PVC size for persistent mounts (x-orcinus-volume-size).
	VolumeSize string `gorm:"size:32" json:"volumeSize"`
	// Path is the ingress path prefix (x-orcinus-path), default "/".
	Path string `gorm:"size:255" json:"path"`
	// HealthCmd is a JSON array exec liveness probe (compose healthcheck test).
	HealthCmd string `gorm:"type:text" json:"healthCmd"`

	// WebhookSecret authorizes push-to-deploy webhooks for this app.
	WebhookSecret string `gorm:"uniqueIndex;size:64" json:"-"`
	// AutoDeploy enables deploying automatically when a webhook fires.
	AutoDeploy bool `json:"autoDeploy"`
}

// Database is a one-click managed database instance.
type Database struct {
	Base
	ProjectID  string `gorm:"index;size:36;not null" json:"projectId"`
	Name       string `gorm:"size:255;not null" json:"name"`
	// OrcinusProject isolates this unit's cluster resources (own prune scope).
	OrcinusProject string `gorm:"uniqueIndex;size:255" json:"orcinusProject"`
	Engine     string `gorm:"size:32;not null" json:"engine"` // postgres|mysql|mariadb|mongo|redis
	Version    string `gorm:"size:32" json:"version"`
	Username   string `gorm:"size:255" json:"username"`
	Password   string `gorm:"size:255" json:"-"`
	DBName     string `gorm:"size:255" json:"dbName"`
	VolumeSize string `gorm:"size:32" json:"volumeSize"`
	Port       int    `json:"port"`
	// Host is the in-cluster DNS name other services use to reach it.
	Host string `gorm:"size:255" json:"host"`
}

// DeployStatus is the lifecycle state of a Deployment.
type DeployStatus string

const (
	DeployPending DeployStatus = "pending"
	DeployRunning DeployStatus = "running"
	DeploySuccess DeployStatus = "success"
	DeployFailed  DeployStatus = "failed"
)

// Deployment records one deploy attempt against the cluster engine.
type Deployment struct {
	Base
	ProjectID     string       `gorm:"index;size:36;not null" json:"projectId"`
	ComposeAppID  string       `gorm:"index;size:36" json:"composeAppId"`
	ApplicationID string       `gorm:"index;size:36" json:"applicationId"`
	Kind          string       `gorm:"size:32" json:"kind"` // compose | application | database
	Status        DeployStatus `gorm:"size:32;not null" json:"status"`
	CommitSHA     string       `gorm:"size:64" json:"commitSha"`
	ImageRef      string       `gorm:"size:512" json:"imageRef"`
	// Source is a snapshot of what was deployed.
	Source    string `gorm:"type:text" json:"source"`
	Applied   int    `json:"applied"`
	Installed string `gorm:"type:text" json:"installed"` // JSON array of plugin names
	Log       string `gorm:"type:text" json:"log"`
	Error     string `gorm:"type:text" json:"error"`
}

// Notification is a configured external notification channel for an org.
type Notification struct {
	Base
	OrganizationID string `gorm:"index;size:36;not null" json:"organizationId"`
	Name           string `gorm:"size:255" json:"name"`
	Type           string `gorm:"size:32;not null" json:"type"` // slack|discord|telegram|webhook|email
	// Config is a JSON object of provider fields (url, token, chatId, smtp…).
	Config    string `gorm:"type:text" json:"-"`
	OnSuccess bool   `json:"onSuccess"`
	OnFailure bool   `json:"onFailure"`
	Enabled   bool   `json:"enabled"`
}

// Backup is a backup configuration + last-run status for a database.
type Backup struct {
	Base
	DatabaseID string `gorm:"index;size:36;not null" json:"databaseId"`
	ProjectID  string `gorm:"index;size:36;not null" json:"projectId"`
	// Cron is a schedule (empty → manual only).
	Cron string `gorm:"size:64" json:"cron"`
	// Destination: local | s3
	Destination string `gorm:"size:32;not null;default:local" json:"destination"`
	// S3Config is a JSON object (endpoint, bucket, accessKey, secretKey, region).
	S3Config   string     `gorm:"type:text" json:"-"`
	Enabled    bool       `json:"enabled"`
	LastRunAt  *time.Time `json:"lastRunAt"`
	LastStatus string     `gorm:"size:32" json:"lastStatus"`
	LastPath   string     `gorm:"size:512" json:"lastPath"`
	LastError  string     `gorm:"type:text" json:"lastError"`
}

// AuditLog records a mutating action for accountability.
type AuditLog struct {
	Base
	UserID  string `gorm:"index;size:36" json:"userId"`
	Email   string `gorm:"size:255" json:"email"`
	Action  string `gorm:"size:64" json:"action"` // METHOD path
	Target  string `gorm:"size:255" json:"target"`
	Status  int    `json:"status"`
}

// ApiToken is a bearer token for programmatic access to kapibara.
type ApiToken struct {
	Base
	UserID    string     `gorm:"index;size:36;not null" json:"userId"`
	Name      string     `gorm:"size:255" json:"name"`
	TokenHash string     `gorm:"uniqueIndex;size:64;not null" json:"-"`
	LastUsed  *time.Time `json:"lastUsed"`
}
