package migration

// Config holds configuration parameters for running a migration.
// Migration is the prefix used for all fields because it avoids conflicts with other environment variables.
type Config struct {
	// MigrationDeadlineMilliseconds is the maximum time for the migrations to complete.
	MigrationDeadlineMilliseconds int `config_format:"snake" config_default:"3600000" validate:"gt=0"`

	// MigrationUnlockDeadlineMilliseconds is the maximum time for a release operation to complete.
	MigrationUnlockDeadlineMilliseconds int `config_format:"snake" config_default:"120000" validate:"gt=0"`

	// MigrationHeartbeatIntervalMilliseconds is how often a heart beat is sent to the migration lock.
	MigrationHeartbeatIntervalMilliseconds int `config_format:"snake" config_default:"10000" validate:"gt=0"`

	// MigrationHeartbeatFailureRetryCount is how many times to retry the heart beat before quitting.
	MigrationHeartbeatFailureRetryCount int `config_format:"snake" config_default:"1" validate:"gte=0"`
}

// migrateConfig is configured by the Option type.
type migrateConfig struct {
	configProvider func() (*Config, error)
}

// Option configures a migrateConfig instance.
type Option func(cfg *migrateConfig)

// WithConfigProvider provides an Option to overwrite the configuration provider.
func WithConfigProvider(callback func() (*Config, error)) Option {
	return func(cfg *migrateConfig) {
		cfg.configProvider = callback
	}
}
