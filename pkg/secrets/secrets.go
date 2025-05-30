package secrets

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// SecretManager defines the minimum shared functionality for a secret manager
// used by this application.
type SecretManager interface {
	GetSecretValue(ctx context.Context, name string) (string, error)
}

// SecretVersionManager is a secret manager that can manage secret versions.
type SecretVersionManager interface {
	SecretManager

	CreateSecretVersion(ctx context.Context, parent string, data []byte) (string, error)
	DestroySecretVersion(ctx context.Context, name string) error
}

// SecretManagerFunc is a func that returns a secret manager or error.
type SecretManagerFunc func(context.Context, *Config) (SecretManager, error)

// managers is the list of registered secret managers.
var (
	managers     = make(map[string]SecretManagerFunc)
	managersLock sync.RWMutex
)

// RegisterManager registers a new secret manager with the given name. If a
// manager is already registered with the given name, it panics. Managers are
// usually registered via an init function.
func RegisterManager(name string, fn SecretManagerFunc) {
	managersLock.Lock()
	defer managersLock.Unlock()

	if _, ok := managers[name]; ok {
		panic(fmt.Sprintf("secret manager %q is already registered", name))
	}
	managers[name] = fn
}

// RegisteredManagers returns the list of the names of the registered secret
// managers.
func RegisteredManagers() []string {
	managersLock.RLock()
	defer managersLock.RUnlock()

	list := make([]string, 0, len(managers))
	for k := range managers {
		list = append(list, k)
	}
	sort.Strings(list)
	return list
}

// SecretManagerFor returns the secret manager with the given name, or an error
// if one does not exist.
func SecretManagerFor(ctx context.Context, cfg *Config) (SecretManager, error) {
	managersLock.RLock()
	defer managersLock.RUnlock()

	name := cfg.Type
	fn, ok := managers[name]
	if !ok {
		return nil, fmt.Errorf("unknown or uncompiled secret manager %q", name)
	}
	return fn(ctx, cfg)
}
