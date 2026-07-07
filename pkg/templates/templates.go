// Package templates is a catalog of one-click application templates. Each
// template is a docker-compose source (with x-orcinus-* hints) parameterized by
// simple {{.KEY}} placeholders that kapibara fills in before deploying.
package templates

import (
	"bytes"
	"fmt"
	"text/template"
)

// Param describes a template input.
type Param struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Default     string `json:"default"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"`
	Description string `json:"description"`
}

// Template is a catalog entry.
type Template struct {
	Name        string  `json:"name"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	Params      []Param `json:"params"`
	Compose     string  `json:"-"` // raw compose with {{.KEY}} placeholders
}

// Render fills the template's compose with values (falling back to defaults),
// returning the deployable source. Missing required params are an error.
func (t Template) Render(values map[string]string) (string, error) {
	data := map[string]string{}
	for _, p := range t.Params {
		v := values[p.Key]
		if v == "" {
			v = p.Default
		}
		if v == "" && p.Required {
			return "", fmt.Errorf("missing required parameter %q", p.Key)
		}
		data[p.Key] = v
	}
	tmpl, err := template.New(t.Name).Option("missingkey=error").Parse(t.Compose)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Catalog is the built-in template catalog.
var Catalog = []Template{
	{
		Name: "wordpress", Title: "WordPress", Category: "CMS",
		Description: "WordPress with a MySQL database.",
		Params: []Param{
			{Key: "DOMAIN", Label: "Domain", Required: true, Description: "Public hostname"},
			{Key: "DB_PASSWORD", Label: "DB Password", Default: "changeme", Secret: true, Required: true},
		},
		Compose: `services:
  wordpress:
    image: wordpress:6
    ports: ["80"]
    environment:
      WORDPRESS_DB_HOST: wp-db
      WORDPRESS_DB_USER: wordpress
      WORDPRESS_DB_PASSWORD: "{{.DB_PASSWORD}}"
      WORDPRESS_DB_NAME: wordpress
    x-orcinus-expose: ingress
    x-orcinus-host: "{{.DOMAIN}}"
    x-orcinus-secret: [WORDPRESS_DB_PASSWORD]
  wp-db:
    image: mysql:8
    environment:
      MYSQL_DATABASE: wordpress
      MYSQL_USER: wordpress
      MYSQL_PASSWORD: "{{.DB_PASSWORD}}"
      MYSQL_ROOT_PASSWORD: "{{.DB_PASSWORD}}"
    x-orcinus-controller: statefulset
    x-orcinus-volume-size: 5Gi
    x-orcinus-secret: [MYSQL_PASSWORD, MYSQL_ROOT_PASSWORD]
    volumes: ["wp-db-data:/var/lib/mysql"]
volumes:
  wp-db-data:
`,
	},
	{
		Name: "redis", Title: "Redis", Category: "Database",
		Description: "A single Redis instance with persistent storage.",
		Params: []Param{
			{Key: "VERSION", Label: "Version", Default: "7"},
		},
		Compose: `services:
  redis:
    image: redis:{{.VERSION}}
    x-orcinus-controller: statefulset
    x-orcinus-volume-size: 1Gi
    volumes: ["redis-data:/data"]
volumes:
  redis-data:
`,
	},
	{
		Name: "n8n", Title: "n8n", Category: "Automation",
		Description: "n8n workflow automation, exposed via ingress.",
		Params: []Param{
			{Key: "DOMAIN", Label: "Domain", Required: true},
		},
		Compose: `services:
  n8n:
    image: n8nio/n8n:latest
    ports: ["5678"]
    environment:
      N8N_PORT: "5678"
      N8N_HOST: "{{.DOMAIN}}"
    x-orcinus-expose: ingress
    x-orcinus-host: "{{.DOMAIN}}"
    x-orcinus-volume-size: 2Gi
    volumes: ["n8n-data:/home/node/.n8n"]
volumes:
  n8n-data:
`,
	},
}

// Find returns the template with the given name.
func Find(name string) (Template, bool) {
	for _, t := range Catalog {
		if t.Name == name {
			return t, true
		}
	}
	return Template{}, false
}
