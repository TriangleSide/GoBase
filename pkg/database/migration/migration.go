package migration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/TriangleSide/GoBase/pkg/config"
	"github.com/TriangleSide/GoBase/pkg/logger"
	"github.com/TriangleSide/GoBase/pkg/validation"
)

// Status represents the status of a persisted migration.
type Status string

const (
	Pending   Status = "PENDING"
	Started   Status = "STARTED"
	Failed    Status = "FAILED"
	Completed Status = "COMPLETED"
)

// PersistedStatus is the data stored in the migration table.
type PersistedStatus struct {
	Order  Order  `validate:"gte=0"`
	Status Status `validate:"oneof=PENDING RUNNING FAILED COMPLETED"`
}

// Runner defines the functions needed to perform a migration.
type Runner interface {
	// AcquireDBLock must acquire a database wide lock.
	// It is used in conjunction with EnsureDataStores and ReleaseDBLock.
	AcquireDBLock(context.Context) error

	// EnsureDataStores must ensure the migration data stores (collections, tables, ...) are created.
	// There should be two data stores, one for the migration lock, and one migration status.
	EnsureDataStores(context.Context) error

	// ReleaseDBLock must release the DB lock acquired by AcquireDBLock.
	ReleaseDBLock(context.Context) error

	// AcquireMigrationLock must acquire a migration lock.
	// This is to ensure only one migrator can run at any given time.
	AcquireMigrationLock(context.Context) error

	// MigrationLockHeartbeat is called on a configurable frequency.
	// It is meant to maintain the lock acquired with AcquireMigrationLock.
	MigrationLockHeartbeat(context.Context) error

	// ListStatuses returns data previously stored with PersistStatus.
	ListStatuses(context.Context) ([]PersistedStatus, error)

	// PersistStatus stores or override the status of a migration.
	// Order must be unique in the data store.
	PersistStatus(context.Context, Order, Status) error

	// ReleaseMigrationLock must release the migration lock acquired with AcquireMigrationLock.
	ReleaseMigrationLock(context.Context) error
}

// Migrate orchestrates database migrations using the provided Runner and options.
func Migrate(runner Runner, opts ...Option) (returnErr error) {
	migrateCfg := &migrateConfig{
		configProvider: func() (*Config, error) {
			return config.ProcessAndValidate[Config]()
		},
	}
	for _, opt := range opts {
		opt(migrateCfg)
	}

	cfg, err := migrateCfg.configProvider()
	if err != nil {
		return fmt.Errorf("failed to get the migration configuration (%w)", err)
	}

	var releaseMigrationLockErr error = nil
	releaseMigrationLockWG := sync.WaitGroup{}

	ctxDeadline := time.Now().Add(time.Millisecond * time.Duration(cfg.MigrationDeadlineMilliseconds))
	ctx, cancel := context.WithDeadline(context.Background(), ctxDeadline)
	defer func() {
		cancel()
		releaseMigrationLockWG.Wait()
		if releaseMigrationLockErr != nil {
			if returnErr != nil {
				returnErr = fmt.Errorf("%w and %w", returnErr, releaseMigrationLockErr)
			} else {
				returnErr = releaseMigrationLockErr
			}
		}
	}()

	if err = ensureDataStores(ctx, runner, cfg); err != nil {
		return fmt.Errorf("failed to ensure the data stores are created (%w)", err)
	}

	if err = runner.AcquireMigrationLock(ctx); err != nil {
		return fmt.Errorf("failed to acquire the migration lock (%w)", err)
	}

	releaseMigrationLockWG.Add(1)
	go func() {
		defer releaseMigrationLockWG.Done()
		if releaseMigrationLockErr = heartbeatAndRelease(ctx, runner, cfg); releaseMigrationLockErr != nil {
			cancel()
		}
	}()

	var migrationsToRun []*Registration
	if migrationsToRun, err = listMigrationsToRun(ctx, runner); err != nil {
		return fmt.Errorf("failed to list the migrations to run (%w)", err)
	}

	if err = runMigrations(ctx, migrationsToRun, runner); err != nil {
		return fmt.Errorf("error while running migrations (%w)", err)
	}

	return nil
}

// ensureDataStores acquires a DB lock, create the migration data stores, and releases the DB lock.
func ensureDataStores(ctx context.Context, runner Runner, cfg *Config) (returnErr error) {
	if err := runner.AcquireDBLock(ctx); err != nil {
		return fmt.Errorf("failed to lock the database (%w)", err)
	}

	defer func() {
		releaseDeadline := time.Now().Add(time.Millisecond * time.Duration(cfg.MigrationUnlockDeadlineMilliseconds))
		releaseCtx, releaseCancel := context.WithDeadline(context.Background(), releaseDeadline)
		defer releaseCancel()
		if releaseErr := runner.ReleaseDBLock(releaseCtx); releaseErr != nil {
			if returnErr != nil {
				returnErr = fmt.Errorf("%w and failed to release the database lock (%w)", returnErr, releaseErr)
			} else {
				returnErr = fmt.Errorf("failed to release the database lock (%w)", releaseErr)
			}
		}
	}()

	if err := runner.EnsureDataStores(ctx); err != nil {
		return fmt.Errorf("failed to ensure the data stores are created (%w)", err)
	}

	return nil
}

// heartbeatAndRelease calls runner.MigrationLockHeartbeat on a configured frequency.
// Once the context is canceled, it calls ReleaseMigrationLock.
func heartbeatAndRelease(ctx context.Context, runner Runner, cfg *Config) (returnErr error) {
	defer func() {
		releaseDeadline := time.Now().Add(time.Millisecond * time.Duration(cfg.MigrationUnlockDeadlineMilliseconds))
		releaseCtx, releaseCancel := context.WithDeadline(context.Background(), releaseDeadline)
		defer releaseCancel()
		if releaseErr := runner.ReleaseMigrationLock(releaseCtx); releaseErr != nil {
			if returnErr != nil {
				returnErr = fmt.Errorf("%w and failed to release the migration lock (%w)", returnErr, releaseErr)
			} else {
				returnErr = fmt.Errorf("failed to release the migration lock (%w)", releaseErr)
			}
		}
	}()

	heartbeatInterval := time.Millisecond * time.Duration(cfg.MigrationHeartbeatIntervalMilliseconds)
	heartbeatTicker := time.NewTicker(heartbeatInterval)
	defer heartbeatTicker.Stop()

	var heartbeatErr error = nil
	var successiveHeartbeatFailureCount = 0

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeatTicker.C:
			if heartbeatErr = runner.MigrationLockHeartbeat(ctx); heartbeatErr != nil {
				successiveHeartbeatFailureCount++
			} else {
				successiveHeartbeatFailureCount = 0
			}
		}
		if successiveHeartbeatFailureCount > cfg.MigrationHeartbeatFailureRetryCount {
			return fmt.Errorf("heartbeat failed %d time(s) with latest error of (%w)", successiveHeartbeatFailureCount, heartbeatErr)
		}
	}
}

// listMigrationsToRun returns a list of registered migrations to run.
func listMigrationsToRun(ctx context.Context, runner Runner) ([]*Registration, error) {
	persistedStatuses, err := runner.ListStatuses(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list the persisted statuses (%w)", err)
	}
	orderToPersistedStatus := make(map[Order]Status)
	for _, persistedStatus := range persistedStatuses {
		if err := validation.Struct(persistedStatus); err != nil {
			return nil, fmt.Errorf("failed while validating the persisted status (%w)", err)
		}
		if _, alreadyFound := orderToPersistedStatus[persistedStatus.Order]; alreadyFound {
			return nil, fmt.Errorf("found two persisted statuses with order %d", persistedStatus.Order)
		}
		orderToPersistedStatus[persistedStatus.Order] = persistedStatus.Status
	}

	latestCompletedMigration := Order(0)
	migrationsToRun := make([]*Registration, 0)
	allRegisteredMigrations := orderedRegistrations()

	for _, registeredMigration := range allRegisteredMigrations {
		if !registeredMigration.Enabled {
			continue
		}
		migrationStatus, migrationStatusFound := orderToPersistedStatus[registeredMigration.Order]
		delete(orderToPersistedStatus, registeredMigration.Order)
		if migrationStatusFound && migrationStatus == Completed {
			logger.Debugf(ctx, "Registration with order %d already completed. Skipping.", registeredMigration.Order)
			if registeredMigration.Order > latestCompletedMigration {
				latestCompletedMigration = registeredMigration.Order
			}
		} else if migrationStatusFound {
			logger.Debugf(ctx, "Will attempt to run the migration with order %d and status %s again.", registeredMigration.Order, migrationStatus)
			migrationsToRun = append(migrationsToRun, registeredMigration)
		} else {
			logger.Debugf(ctx, "New migration with order %d found.", registeredMigration.Order)
			migrationsToRun = append(migrationsToRun, registeredMigration)
		}
	}

	if len(orderToPersistedStatus) != 0 {
		return nil, fmt.Errorf("found persisted migration(s) that are not in the registry (%+v)", orderToPersistedStatus)
	}

	for _, migrationToRun := range migrationsToRun {
		if migrationToRun.Order < latestCompletedMigration {
			return nil, fmt.Errorf("cannot run migrations out of order (found %d but latest completed is %d)", migrationToRun.Order, latestCompletedMigration)
		}
	}

	return migrationsToRun, nil
}

// runMigrations first persists the statuses of the migration to run as PENDING.
// Then it attempts to run the migrations while keeping the persisted statuses updated.
func runMigrations(ctx context.Context, migrationsToRun []*Registration, runner Runner) error {
	for _, registered := range migrationsToRun {
		if err := runner.PersistStatus(ctx, registered.Order, Pending); err != nil {
			return fmt.Errorf("failed to persist the status %s for the migration order %d (%w)", Pending, registered.Order, err)
		}
	}

	for _, registered := range migrationsToRun {
		ctx = logger.WithField(ctx, "order", registered.Order)
		logger.Debug(ctx, "Starting migration.")
		startTime := time.Now()
		if err := runner.PersistStatus(ctx, registered.Order, Started); err != nil {
			return fmt.Errorf("failed to persist the status %s for the migration order %d (%w)", Started, registered.Order, err)
		}
		if err := registered.Migrate(ctx); err != nil {
			err = fmt.Errorf("failed to complete the migration with order %d (%w)", registered.Order, err)
			if failedStatusErr := runner.PersistStatus(ctx, registered.Order, Failed); failedStatusErr != nil {
				return fmt.Errorf("%w and failed to persist its status to %s (%w)", err, Failed, failedStatusErr)
			}
			return err
		}
		if err := runner.PersistStatus(ctx, registered.Order, Completed); err != nil {
			return fmt.Errorf("failed to persist the status %s for the migration order %d (%w)", Completed, registered.Order, err)
		}
		logger.Debugf(ctx, "Migration finished in %s.", time.Now().Sub(startTime))
	}

	return nil
}
