package migration

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/TriangleSide/GoBase/pkg/config"
	"github.com/TriangleSide/GoBase/pkg/test/assert"
)

type runnerRecorder struct {
	Operations          []string
	PersistedMigrations []PersistedStatus

	Heartbeat            chan struct{}
	HeartbeatCount       int
	MigrationUnlockCount int
	FailOnStatus         string

	AcquireDBLockError          error
	EnsureDataStoresError       error
	ReleaseDBLockError          error
	MigrationLockError          error
	MigrationLockHeartbeatError error
	ListStatusesError           error
	PersistStatusError          error
	ReleaseMigrationLockError   error
}

func (r *runnerRecorder) AcquireDBLock(ctx context.Context) error {
	r.Operations = append(r.Operations, "AcquireDBLock()")
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return r.AcquireDBLockError
}

func (r *runnerRecorder) EnsureDataStores(ctx context.Context) error {
	r.Operations = append(r.Operations, "EnsureDataStores()")
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return r.EnsureDataStoresError
}

func (r *runnerRecorder) ReleaseDBLock(ctx context.Context) error {
	r.Operations = append(r.Operations, "ReleaseDBLock()")
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return r.ReleaseDBLockError
}

func (r *runnerRecorder) AcquireMigrationLock(ctx context.Context) error {
	r.Operations = append(r.Operations, "AcquireMigrationLock()")
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return r.MigrationLockError
}

func (r *runnerRecorder) MigrationLockHeartbeat(ctx context.Context) error {
	// Not recorded in operations because of races with the heartbeat go routine.
	r.HeartbeatCount++
	if r.Heartbeat != nil {
		r.Heartbeat <- struct{}{}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return r.MigrationLockHeartbeatError
}

func (r *runnerRecorder) ListStatuses(ctx context.Context) ([]PersistedStatus, error) {
	r.Operations = append(r.Operations, "ListStatuses()")
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return r.PersistedMigrations, r.ListStatusesError
}

func (r *runnerRecorder) PersistStatus(ctx context.Context, order Order, status Status) error {
	r.Operations = append(r.Operations, fmt.Sprintf("PersistStatus(order=%d, status=%s)", order, status))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if string(status) == r.FailOnStatus {
		return errors.New("fail on " + string(status))
	}
	return r.PersistStatusError
}

func (r *runnerRecorder) ReleaseMigrationLock(ctx context.Context) error {
	// Not recorded in operations because of races with the heartbeat go routine.
	r.MigrationUnlockCount++
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return r.ReleaseMigrationLockError
}

func TestMigrate(t *testing.T) {
	standardRegisteredMigration := func(runner *runnerRecorder, order Order) *Registration {
		return &Registration{
			Order: order,
			Migrate: func(ctx context.Context) error {
				runner.Operations = append(runner.Operations, fmt.Sprintf("Migration%d.Migrate()", order))
				return ctx.Err()
			},
			Enabled: true,
		}
	}

	tests := []struct {
		name          string
		runner        *runnerRecorder
		setupRegistry func(runner *runnerRecorder)
		expectedErr   string
		expectedOps   []string
		options       []Option
		asserts       func(t *testing.T, runner *runnerRecorder)
	}{
		{
			name:          "when configProvider fails it should return an error",
			runner:        &runnerRecorder{},
			setupRegistry: func(runner *runnerRecorder) {},
			expectedErr:   "failed to get the migration configuration",
			expectedOps:   nil,
			options: []Option{
				WithConfigProvider(func() (*Config, error) {
					return nil, errors.New("configProvider error")
				}),
			},
		},
		{
			name:   "when everything works as expected it should run migrations successfully",
			runner: &runnerRecorder{},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(standardRegisteredMigration(runner, Order(1)))
				MustRegister(standardRegisteredMigration(runner, Order(2)))
				MustRegister(&Registration{
					Order: 3,
					Migrate: func(ctx context.Context) error {
						runner.Operations = append(runner.Operations, "Migration3.Migrate()")
						return ctx.Err()
					},
					Enabled: false,
				})
			},
			expectedErr: "",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
				"PersistStatus(order=1, status=PENDING)",
				"PersistStatus(order=2, status=PENDING)",
				"PersistStatus(order=1, status=STARTED)",
				"Migration1.Migrate()",
				"PersistStatus(order=1, status=COMPLETED)",
				"PersistStatus(order=2, status=STARTED)",
				"Migration2.Migrate()",
				"PersistStatus(order=2, status=COMPLETED)",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when AcquireDBLock fails it should return an error",
			runner: &runnerRecorder{
				AcquireDBLockError: errors.New("AcquireDBLock error"),
			},
			setupRegistry: func(runner *runnerRecorder) {},
			expectedErr:   "failed to lock the database (AcquireDBLock error)",
			expectedOps: []string{
				"AcquireDBLock()",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 0)
			},
		},
		{
			name: "when EnsureDataStores fails it should return an error",
			runner: &runnerRecorder{
				EnsureDataStoresError: errors.New("EnsureDataStores error"),
			},
			setupRegistry: func(runner *runnerRecorder) {},
			expectedErr:   "failed to ensure the data stores are created (EnsureDataStores error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 0)
			},
		},
		{
			name: "when EnsureDataStores and ReleaseDBLock fails it should return an error",
			runner: &runnerRecorder{
				EnsureDataStoresError: errors.New("EnsureDataStores error"),
				ReleaseDBLockError:    errors.New("ReleaseDBLockError error"),
			},
			setupRegistry: func(runner *runnerRecorder) {},
			expectedErr:   "failed to ensure the data stores are created (EnsureDataStores error) and failed to release the database lock (ReleaseDBLockError error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 0)
			},
		},
		{
			name: "when ReleaseDBLock fails it should return an error",
			runner: &runnerRecorder{
				ReleaseDBLockError: errors.New("ReleaseDBLock error"),
			},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(standardRegisteredMigration(runner, Order(1)))
			},
			expectedErr: "failed to release the database lock (ReleaseDBLock error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 0)
			},
		},
		{
			name: "when AcquireMigrationLock fails it should return an error",
			runner: &runnerRecorder{
				MigrationLockError: errors.New("AcquireMigrationLock error"),
			},
			setupRegistry: func(runner *runnerRecorder) {},
			expectedErr:   "failed to acquire the migration lock (AcquireMigrationLock error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 0)
			},
		},
		{
			name: "when ListStatuses fails it should return an error",
			runner: &runnerRecorder{
				ListStatusesError: errors.New("ListStatuses error"),
			},
			setupRegistry: func(runner *runnerRecorder) {},
			expectedErr:   "failed to list the persisted statuses (ListStatuses error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when persisted migrations have invalid status it should return an error",
			runner: &runnerRecorder{
				PersistedMigrations: []PersistedStatus{
					{Order: 1, Status: "INVALID"},
				},
			},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(standardRegisteredMigration(runner, Order(1)))
			},
			expectedErr: "the value 'INVALID' is not one of the allowed values",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when there are failed migrations it should try them again",
			runner: &runnerRecorder{
				PersistedMigrations: []PersistedStatus{
					{Order: 1, Status: Failed},
				},
			},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(standardRegisteredMigration(runner, Order(1)))
			},
			expectedErr: "",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
				"PersistStatus(order=1, status=PENDING)",
				"PersistStatus(order=1, status=STARTED)",
				"Migration1.Migrate()",
				"PersistStatus(order=1, status=COMPLETED)",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name:   "when registered.Migrate fails it should return an error",
			runner: &runnerRecorder{},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(&Registration{
					Order: 1,
					Migrate: func(ctx context.Context) error {
						runner.Operations = append(runner.Operations, "Migration1.Migrate()")
						return errors.New("migrate error")
					},
					Enabled: true,
				})
			},
			expectedErr: "failed to complete the migration with order 1 (migrate error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
				"PersistStatus(order=1, status=PENDING)",
				"PersistStatus(order=1, status=STARTED)",
				"Migration1.Migrate()",
				"PersistStatus(order=1, status=FAILED)",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when setting the status to PENDING fails it should return an error",
			runner: &runnerRecorder{
				FailOnStatus: string(Pending),
			},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(standardRegisteredMigration(runner, Order(1)))
			},
			expectedErr: "failed to persist the status PENDING for the migration order 1 (fail on PENDING)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
				"PersistStatus(order=1, status=PENDING)",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when there are persisted migrations not in the registry it return an error",
			runner: &runnerRecorder{
				PersistedMigrations: []PersistedStatus{
					{Order: 1, Status: Completed},
				},
			},
			setupRegistry: func(runner *runnerRecorder) {},
			expectedErr:   "found persisted migration(s) that are not in the registry",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when ReleaseMigrationLock fails it should return an error",
			runner: &runnerRecorder{
				ReleaseMigrationLockError: errors.New("ReleaseMigrationLock error"),
			},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(standardRegisteredMigration(runner, Order(1)))
			},
			expectedErr: "failed to release the migration lock (ReleaseMigrationLock error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
				"PersistStatus(order=1, status=PENDING)",
				"PersistStatus(order=1, status=STARTED)",
				"Migration1.Migrate()",
				"PersistStatus(order=1, status=COMPLETED)",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when MigrationLockHeartbeat fails continuously it should cancel context and stop migrations",
			runner: &runnerRecorder{
				MigrationLockHeartbeatError: errors.New("heartbeat error"),
			},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(&Registration{
					Order: 1,
					Migrate: func(ctx context.Context) error {
						<-ctx.Done()
						runner.Operations = append(runner.Operations, "Migration1.Migrate()")
						return ctx.Err()
					},
					Enabled: true,
				})
			},
			expectedErr: "failed to complete the migration with order 1 (context canceled) and failed to persist its status to FAILED (context canceled)) and heartbeat failed 3 time(s) with latest error of (heartbeat error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
				"PersistStatus(order=1, status=PENDING)",
				"PersistStatus(order=1, status=STARTED)",
				"Migration1.Migrate()",
				"PersistStatus(order=1, status=FAILED)",
			},
			options: []Option{
				WithConfigProvider(func() (*Config, error) {
					cfg, _ := config.Process[Config]()
					cfg.MigrationHeartbeatIntervalMilliseconds = 1
					cfg.MigrationHeartbeatFailureRetryCount = 2
					return cfg, nil
				}),
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.HeartbeatCount, 3)
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when MigrationLockHeartbeat fails continuously and ReleaseMigrationLock fails it should cancel context and stop migrations",
			runner: &runnerRecorder{
				MigrationLockHeartbeatError: errors.New("heartbeat error"),
				ReleaseMigrationLockError:   errors.New("ReleaseMigrationLockError error"),
			},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(&Registration{
					Order: 1,
					Migrate: func(ctx context.Context) error {
						<-ctx.Done()
						runner.Operations = append(runner.Operations, "Migration1.Migrate()")
						return ctx.Err()
					},
					Enabled: true,
				})
			},
			expectedErr: "failed to complete the migration with order 1 (context canceled) and failed to persist its status to FAILED (context canceled)) and heartbeat failed 3 time(s) with latest error of (heartbeat error) and failed to release the migration lock (ReleaseMigrationLockError error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
				"PersistStatus(order=1, status=PENDING)",
				"PersistStatus(order=1, status=STARTED)",
				"Migration1.Migrate()",
				"PersistStatus(order=1, status=FAILED)",
			},
			options: []Option{
				WithConfigProvider(func() (*Config, error) {
					cfg, _ := config.Process[Config]()
					cfg.MigrationHeartbeatIntervalMilliseconds = 1
					cfg.MigrationHeartbeatFailureRetryCount = 2
					return cfg, nil
				}),
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.HeartbeatCount, 3)
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when MigrationLockHeartbeat succeeds it should not prevent progress",
			runner: &runnerRecorder{
				Heartbeat: make(chan struct{}),
			},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(&Registration{
					Order: 1,
					Migrate: func(ctx context.Context) error {
						<-runner.Heartbeat
						runner.Operations = append(runner.Operations, "Migration1.Migrate()")
						return ctx.Err()
					},
					Enabled: true,
				})
			},
			expectedErr: "",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
				"PersistStatus(order=1, status=PENDING)",
				"PersistStatus(order=1, status=STARTED)",
				"Migration1.Migrate()",
				"PersistStatus(order=1, status=COMPLETED)",
			},
			options: []Option{
				WithConfigProvider(func() (*Config, error) {
					cfg, _ := config.Process[Config]()
					cfg.MigrationHeartbeatIntervalMilliseconds = 10
					return cfg, nil
				}),
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.True(t, runner.HeartbeatCount > 0)
			},
		},
		{
			name: "when there are two migration statuses with the same order it should return an error",
			runner: &runnerRecorder{
				PersistedMigrations: []PersistedStatus{
					{Order: 1, Status: Completed},
					{Order: 1, Status: Completed},
				},
			},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(standardRegisteredMigration(runner, Order(1)))
			},
			expectedErr: "found two persisted statuses with order 1",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when a migration with a completed status is encountered it should skip the migration",
			runner: &runnerRecorder{
				PersistedMigrations: []PersistedStatus{
					{Order: 1, Status: Completed},
				},
			},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(standardRegisteredMigration(runner, Order(1)))
			},
			expectedErr: "",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when PersistStatus fails when setting status to STARTED it should return an error",
			runner: &runnerRecorder{
				FailOnStatus: string(Started),
			},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(standardRegisteredMigration(runner, Order(1)))
			},
			expectedErr: "failed to persist the status STARTED for the migration order 1 (fail on STARTED)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
				"PersistStatus(order=1, status=PENDING)",
				"PersistStatus(order=1, status=STARTED)",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when PersistStatus fails when setting status to COMPLETED it should return an error",
			runner: &runnerRecorder{
				FailOnStatus: string(Completed),
			},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(standardRegisteredMigration(runner, Order(1)))
			},
			expectedErr: "failed to persist the status COMPLETED for the migration order 1 (fail on COMPLETED)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
				"PersistStatus(order=1, status=PENDING)",
				"PersistStatus(order=1, status=STARTED)",
				"Migration1.Migrate()",
				"PersistStatus(order=1, status=COMPLETED)",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when a persisted status is out of sequence with the registered migrations it should return an error",
			runner: &runnerRecorder{
				PersistedMigrations: []PersistedStatus{
					{Order: 2, Status: Completed},
				},
			},
			setupRegistry: func(runner *runnerRecorder) {
				MustRegister(standardRegisteredMigration(runner, Order(1)))
				MustRegister(standardRegisteredMigration(runner, Order(2)))
			},
			expectedErr: "cannot run migrations out of order (found 1 but latest completed is 2)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
			},
			asserts: func(t *testing.T, runner *runnerRecorder) {
				t.Helper()
				assert.Equals(t, runner.MigrationUnlockCount, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry.Clear()
			if tt.setupRegistry != nil {
				tt.setupRegistry(tt.runner)
			}
			err := Migrate(tt.runner, tt.options...)
			if tt.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorPart(t, err, tt.expectedErr)
			}
			assert.Equals(t, tt.expectedOps, tt.runner.Operations)
			if tt.asserts != nil {
				tt.asserts(t, tt.runner)
			}
		})
	}
}
