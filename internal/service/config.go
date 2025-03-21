package service

var (
// _ setup.BlobstoreConfigProvider     = (*Config)(nil)
// _ setup.DatabaseConfigProvider      = (*Config)(nil)
// _ setup.KeyManagerConfigProvider    = (*Config)(nil)
// _ setup.SecretManagerConfigProvider = (*Config)(nil)
)

type Config struct {
	// Database      database.Config
	// KeyManager    keys.Config
	// SecretManager secrets.Config
	// Storage       storage.Config
	ProfilingEnabled bool   `env:"PROFILING_ENABLED, default=false"`
	Port             string `env:"PORT, default=8080"`
}

// func (c *Config) DatabaseConfig() *database.Config {
// 	return &c.Database
// }

// func (c *Config) KeyManagerConfig() *keys.Config {
// 	return &c.KeyManager
// }

// func (c *Config) SecretManagerConfig() *secrets.Config {
// 	return &c.SecretManager
// }

// func (c *Config) BlobstoreConfig() *storage.Config {
// 	return &c.Storage
// }

// func (c *Config) TemplateRenderer() (*template.Template, error) {
// 	tmpl, err := template.New("").
// 		Option("missingkey=zero").
// 		Funcs(TemplateFuncMap).
// 		ParseFS(templatesFS, "templates/*.html")
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to parse templates from fs: %w", err)
// 	}
// 	return tmpl, nil
// }
