package payment

import (
	"time"
)

// // Compile-time check to assert this config matches requirements.
// var (
// 	_ setup.DatabaseConfigProvider              = (*Config)(nil)
// 	_ setup.SecretManagerConfigProvider         = (*Config)(nil)
// 	_ setup.ObservabilityExporterConfigProvider = (*Config)(nil)
// )

// Config is the configuration for the federation components (data sent to other servers).
type Config struct {
	// Database              database.Config
	// SecretManager         secrets.Config
	// ObservabilityExporter observability.Config

	Port           string        `env:"PORT, default=8080"`
	MaxRecords     uint32        `env:"MAX_RECORDS, default=500"`
	Timeout        time.Duration `env:"RPC_TIMEOUT, default=5m"`
	TruncateWindow time.Duration `env:"TRUNCATE_WINDOW, default=1h"`

	// AllowAnyClient, if true, removes authentication requirements on the
	// federation endpoint. In practice, this is only useful in local testing.
	AllowAnyClient bool `env:"ALLOW_ANY_CLIENT"`

	// TLSCertFile is the certificate file to use if TLS encryption is enabled on
	// the server. If present, TLSKeyFile must also be present. These settings
	// should be left blank on Managed Cloud Run where the TLS termination is
	// handled by the environment.
	TLSCertFile string `env:"TLS_CERT_FILE"`
	TLSKeyFile  string `env:"TLS_KEY_FILE"`
}

// func (c *Config) DatabaseConfig() *database.Config {
// 	return &c.Database
// }

// func (c *Config) SecretManagerConfig() *secrets.Config {
// 	return &c.SecretManager
// }

// func (c *Config) ObservabilityExporterConfig() *observability.Config {
// 	return &c.ObservabilityExporter
// }
