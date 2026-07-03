package deployer

import (
	"context"
	"encoding/json"

	"github.com/orcinustools/kapibara/pkg/compose"
	"github.com/orcinustools/kapibara/pkg/database"
	"github.com/orcinustools/kapibara/pkg/orcinus"
	"github.com/orcinustools/kapibara/pkg/store"
)

// DeployDatabase provisions a managed database as a statefulset via orcinus.
func (d *Deployer) DeployDatabase(ctx context.Context, db *store.Database, project *store.Project) (*store.Deployment, error) {
	spec := database.Spec{
		Engine:     database.Engine(db.Engine),
		Name:       sanitize(db.Name),
		Version:    db.Version,
		Username:   db.Username,
		Password:   db.Password,
		Database:   db.DBName,
		VolumeSize: db.VolumeSize,
	}
	svc, volName, err := spec.Service()
	if err != nil {
		return nil, err
	}
	source, err := compose.Project{
		Services: []compose.Service{svc},
		Volumes:  []string{volName},
	}.Render()
	if err != nil {
		return nil, err
	}

	dep := &store.Deployment{
		ProjectID: project.ID,
		Kind:      "database",
		Status:    store.DeployRunning,
		Source:    source,
	}
	if err := d.Store.CreateDeployment(dep); err != nil {
		return nil, err
	}

	target := db.OrcinusProject
	if target == "" {
		target = project.OrcinusProject
	}
	res, err := d.Orcinus.Deploy(ctx, orcinus.DeployRequest{
		Source:  source,
		Project: target,
		Wait:    true,
	})
	if err != nil {
		dep.Status = store.DeployFailed
		dep.Error = err.Error()
		_ = d.Store.UpdateDeployment(dep)
		return dep, err
	}

	dep.Status = store.DeploySuccess
	dep.Applied = res.Applied
	if b, e := json.Marshal(res.Installed); e == nil {
		dep.Installed = string(b)
	}
	_ = d.Store.UpdateDeployment(dep)

	// The in-cluster host is the service name; other services in the same
	// project reach it directly by this name.
	db.Host = spec.Name
	db.Port = spec.Port()
	_ = d.Store.UpdateDatabase(db)
	return dep, nil
}

// ConnectionString returns the in-cluster URI for a provisioned database.
func ConnectionString(db *store.Database) string {
	spec := database.Spec{
		Engine:   database.Engine(db.Engine),
		Username: db.Username,
		Password: db.Password,
		Database: db.DBName,
	}
	host := db.Host
	if host == "" {
		host = sanitize(db.Name)
	}
	return spec.ConnectionString(host)
}
