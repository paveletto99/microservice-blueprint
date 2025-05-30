package secrets

import (
	"time"
)

// Config represents the config for a secret manager.
type Config struct {
	Type            string        `env:"SECRET_MANAGER, default=IN_MEMORY"`
	SecretsDir      string        `env:"SECRETS_DIR, default=/var/run/secrets"`
	SecretCacheTTL  time.Duration `env:"SECRET_CACHE_TTL, default=5m"`
	SecretExpansion bool          `env:"SECRET_EXPANSION, default=false"`

	// FilesystemRoot is the root path where secrets are managed on the filesystem.
	FilesystemRoot string `env:"SECRET_FILESYSTEM_ROOT"`
}
