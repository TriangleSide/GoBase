package migration

import (
	"github.com/TriangleSide/GoBase/pkg/config"
)

const (
	ConfigPrefix = "MIGRATION"
)

// Config holds parameters for running a migration.
type Config struct {
	// DeadlineMilliseconds is the maximum time for the migrations to complete.
	DeadlineMilliseconds int `config_format:"snake" config_default:"3600000" validate:"gt=0"`

	// UnlockDeadlineMilliseconds is the maximum time for a release operation to complete.
	UnlockDeadlineMilliseconds int `config_format:"snake" config_default:"120000" validate:"gt=0"`

	// HeartbeatIntervalMilliseconds is how often a heart beat is sent to the migration lock.
	HeartbeatIntervalMilliseconds int `config_format:"snake" config_default:"10000" validate:"gt=0"`

	// HeartbeatFailureRetryCount is how many times to retry the heart beat before quitting.
	HeartbeatFailureRetryCount int `config_format:"snake" config_default:"1" validate:"gte=0"`
}

// migrateConfig is configured by the Option type.
type migrateConfig struct {
	configProvider func() (*Config, error)
}

// Option configures a migrateConfig instance.
type Option func(cfg *migrateConfig)

// configure applies options to the default migrateConfig values.
func configure(opts ...Option) *migrateConfig {
	migrateCfg := &migrateConfig{
		configProvider: func() (*Config, error) {
			return config.ProcessAndValidate[Config](config.WithPrefix(ConfigPrefix))
		},
	}
	for _, opt := range opts {
		opt(migrateCfg)
	}
	return migrateCfg
}

// WithConfigProvider provides an Option to overwrite the configuration provider.
func WithConfigProvider(callback func() (*Config, error)) Option {
	return func(cfg *migrateConfig) {
		cfg.configProvider = callback
	}
}
