package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"
)

// dbInfo mirrors the server's database view (dbView).
type dbInfo struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Engine           string `json:"engine"`
	Version          string `json:"version"`
	Username         string `json:"username"`
	DBName           string `json:"dbName"`
	Host             string `json:"host"`
	Port             int    `json:"port"`
	ConnectionString string `json:"connectionString"`
}

// databaseCmd manages one-click managed databases (postgres, mysql, mariadb,
// mongo, redis). Each becomes a StatefulSet + PVC + ClusterIP Service reachable
// in-cluster by its name.
func databaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "db",
		Aliases: []string{"database", "databases"},
		Short:   "Manage one-click databases (postgres|mysql|mariadb|mongo|redis)",
		Long: "Manage one-click managed databases. Examples:\n" +
			"  kapibara db create --project shop --name pg --engine postgres --deploy\n" +
			"  kapibara db create --project shop --name cache --engine redis --deploy\n" +
			"  kapibara db list --project shop\n" +
			"  kapibara db info <dbID>",
	}

	var project string
	list := &cobra.Command{
		Use:   "list",
		Short: "List databases in a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return fmt.Errorf("--project (name or id) is required")
			}
			cfg := loadCLIConfig()
			client := newAPIClient(cfg)
			p, err := resolveProject(cmd.Context(), client, cfg, project, false)
			if err != nil {
				return err
			}
			var out struct {
				Databases []dbInfo `json:"databases"`
			}
			if err := client.do(cmd.Context(), http.MethodGet, "/api/v1/projects/"+p.ID+"/databases", nil, &out); err != nil {
				return err
			}
			if len(out.Databases) == 0 {
				fmt.Println("no databases")
				return nil
			}
			for _, d := range out.Databases {
				where := "(not deployed)"
				if d.Host != "" {
					where = fmt.Sprintf("%s:%d", d.Host, d.Port)
				}
				fmt.Printf("%-38s  %-14s  %-8s  %s\n", d.ID, d.Name, d.Engine, where)
			}
			return nil
		},
	}
	list.Flags().StringVar(&project, "project", "", "project name or id")

	var name, engine, dbVersion, username, password, dbName, volumeSize string
	var deploy bool
	create := &cobra.Command{
		Use:   "create",
		Short: "Create a database (add --deploy to provision it immediately)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" || name == "" || engine == "" {
				return fmt.Errorf("--project, --name and --engine are required")
			}
			cfg := loadCLIConfig()
			client := newAPIClient(cfg)
			p, err := resolveProject(cmd.Context(), client, cfg, project, true)
			if err != nil {
				return err
			}
			body := map[string]any{
				"name": name, "engine": engine, "version": dbVersion,
				"username": username, "password": password, "dbName": dbName, "volumeSize": volumeSize,
			}
			var db dbInfo
			if err := client.do(cmd.Context(), http.MethodPost, "/api/v1/projects/"+p.ID+"/databases", body, &db); err != nil {
				return err
			}
			fmt.Printf("created database %s (%s) id=%s\n", db.Name, db.Engine, db.ID)
			if deploy {
				return deployDatabase(cmd.Context(), client, db.ID)
			}
			fmt.Printf("deploy it with: kapibara db deploy %s\n", db.ID)
			return nil
		},
	}
	create.Flags().StringVar(&project, "project", "", "project name or id")
	create.Flags().StringVar(&name, "name", "", "database name (also its in-cluster host)")
	create.Flags().StringVar(&engine, "engine", "", "postgres | mysql | mariadb | mongo | redis")
	create.Flags().StringVar(&dbVersion, "version", "", "image tag (default per engine)")
	create.Flags().StringVar(&username, "username", "", "username (default: kapibara)")
	create.Flags().StringVar(&password, "password", "", "password (default: generated)")
	create.Flags().StringVar(&dbName, "dbname", "", "initial database name (default: app)")
	create.Flags().StringVar(&volumeSize, "volume-size", "", "PVC size (default: 1Gi)")
	create.Flags().BoolVar(&deploy, "deploy", false, "provision (deploy) the database right after creating")

	deployC := &cobra.Command{
		Use:   "deploy DB_ID",
		Short: "Provision (deploy) a database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newAPIClient(loadCLIConfig())
			return deployDatabase(cmd.Context(), client, args[0])
		},
	}

	info := &cobra.Command{
		Use:   "info DB_ID",
		Short: "Show a database's connection details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newAPIClient(loadCLIConfig())
			var db dbInfo
			if err := client.do(cmd.Context(), http.MethodGet, "/api/v1/databases/"+args[0], nil, &db); err != nil {
				return err
			}
			fmt.Printf("name:       %s\nengine:     %s %s\nhost:       %s\nport:       %d\nusername:   %s\ndatabase:   %s\nconnection: %s\n",
				db.Name, db.Engine, db.Version, db.Host, db.Port, db.Username, db.DBName, db.ConnectionString)
			return nil
		},
	}

	rm := &cobra.Command{
		Use:   "rm DB_ID",
		Short: "Delete a database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := newAPIClient(loadCLIConfig())
			if err := client.do(cmd.Context(), http.MethodDelete, "/api/v1/databases/"+args[0], nil, nil); err != nil {
				return err
			}
			fmt.Printf("✓ deleted database %s\n", args[0])
			return nil
		},
	}

	cmd.AddCommand(list, create, deployC, info, rm)
	return cmd
}

// deployDatabase provisions a database and prints its connection details.
func deployDatabase(ctx context.Context, client *apiClient, id string) error {
	fmt.Printf("deploying database %s…\n", id)
	var out struct {
		ConnectionString string `json:"connectionString"`
		Host             string `json:"host"`
		Port             int    `json:"port"`
		Deployment       struct {
			Status string `json:"status"`
		} `json:"deployment"`
	}
	if err := client.do(ctx, http.MethodPost, "/api/v1/databases/"+id+"/deploy", nil, &out); err != nil {
		return err
	}
	status := out.Deployment.Status
	if status == "" {
		status = "deployed"
	}
	fmt.Printf("✓ %s → %s:%d\n", status, out.Host, out.Port)
	if out.ConnectionString != "" {
		fmt.Printf("  connection: %s\n", out.ConnectionString)
	}
	return nil
}
