package backup

import (
	"time"

	"github.com/paveletto99/microservice-blueprint/internal/setup"
	"github.com/paveletto99/microservice-blueprint/pkg/database"
)

// Compile-time check to assert this config matches requirements.
var (
	_ setup.DatabaseConfigProvider = (*Config)(nil)
	// _ setup.ObservabilityExporterConfigProvider = (*Config)(nil)
	// _ setup.SecretManagerConfigProvider         = (*Config)(nil)
)

// Config represents the configuration and associated environment variables for
// the cleanup components.
type Config struct {
	Database database.Config
	// ObservabilityExporter observability.Config
	// SecretManager         secrets.Config

	Port string `env:"PORT, default=8080"`

	// MinTTL is the minimum amount of time that must elapse between attempting
	// backups. This is used to control whether the pull is actually attempted at
	// the controller layer, independent of the data layer. In effect, it rate
	// limits the number of requests.
	MinTTL time.Duration `env:"BACKUP_MIN_PERIOD, default=5m"`

	// Timeout is the maximum amount of time to wait for a backup operation to
	// complete.
	Timeout time.Duration `env:"BACKUP_TIMEOUT, default=10m"`

	// Bucket is the name of the Cloud Storage bucket where backups should be
	// stored.
	Bucket string `env:"BACKUP_BUCKET, default=sql-backups"`

	// DatabaseInstanceURL is the full self-link of the URL to the SQL instance.
	DatabaseInstanceURL string `env:"BACKUP_DATABASE_INSTANCE_URL, default=projects/my-project/instances/my-instance"`

	// DatabaseName is the name of the database to backup.
	DatabaseName string `env:"BACKUP_DATABASE_NAME, default=my-database"`
}

func (c *Config) DatabaseConfig() *database.Config {
	return &c.Database
}

// func (c *Config) ObservabilityExporterConfig() *observability.Config {
// 	return &c.ObservabilityExporter
// }

// func (c *Config) SecretManagerConfig() *secrets.Config {
// 	return &c.SecretManager
// }
