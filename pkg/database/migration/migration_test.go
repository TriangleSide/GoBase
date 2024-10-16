package migration

import (
	"context"
	"errors"
	"fmt"
	"github.com/TriangleSide/GoBase/pkg/validation"
	"testing"

	"github.com/TriangleSide/GoBase/pkg/config"
	"github.com/TriangleSide/GoBase/pkg/test/assert"
)

type managerRecorder struct {
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

func (r *managerRecorder) AcquireDBLock(ctx context.Context) error {
	r.Operations = append(r.Operations, "AcquireDBLock()")
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return r.AcquireDBLockError
}

func (r *managerRecorder) EnsureDataStores(ctx context.Context) error {
	r.Operations = append(r.Operations, "EnsureDataStores()")
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return r.EnsureDataStoresError
}

func (r *managerRecorder) ReleaseDBLock(ctx context.Context) error {
	r.Operations = append(r.Operations, "ReleaseDBLock()")
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return r.ReleaseDBLockError
}

func (r *managerRecorder) AcquireMigrationLock(ctx context.Context) error {
	r.Operations = append(r.Operations, "AcquireMigrationLock()")
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return r.MigrationLockError
}

func (r *managerRecorder) MigrationLockHeartbeat(ctx context.Context) error {
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

func (r *managerRecorder) ListStatuses(ctx context.Context) ([]PersistedStatus, error) {
	r.Operations = append(r.Operations, "ListStatuses()")
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return r.PersistedMigrations, r.ListStatusesError
}

func (r *managerRecorder) PersistStatus(ctx context.Context, order Order, status Status) error {
	r.Operations = append(r.Operations, fmt.Sprintf("PersistStatus(order=%d, status=%s)", order, status))
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if string(status) == r.FailOnStatus {
		return errors.New("fail on " + string(status))
	}
	return r.PersistStatusError
}

func (r *managerRecorder) ReleaseMigrationLock(ctx context.Context) error {
	// Not recorded in operations because of races with the heartbeat go routine.
	r.MigrationUnlockCount++
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return r.ReleaseMigrationLockError
}

func TestMigrate(t *testing.T) {
	standardRegisteredMigration := func(manager *managerRecorder, order Order) *Registration {
		return &Registration{
			Order: order,
			Migrate: func(ctx context.Context) error {
				manager.Operations = append(manager.Operations, fmt.Sprintf("Migration%d.Migrate()", order))
				return ctx.Err()
			},
			Enabled: true,
		}
	}

	tests := []struct {
		name          string
		manager       *managerRecorder
		setupRegistry func(manager *managerRecorder)
		expectedErr   string
		expectedOps   []string
		options       []Option
		asserts       func(t *testing.T, manager *managerRecorder)
	}{
		{
			name:          "when configProvider fails it should return an error",
			manager:       &managerRecorder{},
			setupRegistry: func(manager *managerRecorder) {},
			expectedErr:   "failed to get the migration configuration",
			expectedOps:   nil,
			options: []Option{
				WithConfigProvider(func() (*Config, error) {
					return nil, errors.New("configProvider error")
				}),
			},
		},
		{
			name:    "when everything works as expected it should run migrations successfully",
			manager: &managerRecorder{},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(standardRegisteredMigration(manager, Order(1)))
				MustRegister(standardRegisteredMigration(manager, Order(2)))
				MustRegister(&Registration{
					Order: 3,
					Migrate: func(ctx context.Context) error {
						manager.Operations = append(manager.Operations, "Migration3.Migrate()")
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
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when AcquireDBLock fails it should return an error",
			manager: &managerRecorder{
				AcquireDBLockError: errors.New("AcquireDBLock error"),
			},
			setupRegistry: func(manager *managerRecorder) {},
			expectedErr:   "failed to acquire the database lock (AcquireDBLock error)",
			expectedOps: []string{
				"AcquireDBLock()",
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 0)
			},
		},
		{
			name: "when EnsureDataStores fails it should return an error",
			manager: &managerRecorder{
				EnsureDataStoresError: errors.New("EnsureDataStores error"),
			},
			setupRegistry: func(manager *managerRecorder) {},
			expectedErr:   "failed to ensure the data stores are created (EnsureDataStores error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 0)
			},
		},
		{
			name: "when EnsureDataStores and ReleaseDBLock fails it should return an error",
			manager: &managerRecorder{
				EnsureDataStoresError: errors.New("EnsureDataStores error"),
				ReleaseDBLockError:    errors.New("ReleaseDBLockError error"),
			},
			setupRegistry: func(manager *managerRecorder) {},
			expectedErr:   "failed to ensure the data stores are created (EnsureDataStores error) and failed to release the database lock (ReleaseDBLockError error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 0)
			},
		},
		{
			name: "when ReleaseDBLock fails it should return an error",
			manager: &managerRecorder{
				ReleaseDBLockError: errors.New("ReleaseDBLock error"),
			},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(standardRegisteredMigration(manager, Order(1)))
			},
			expectedErr: "failed to release the database lock (ReleaseDBLock error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 0)
			},
		},
		{
			name: "when AcquireMigrationLock fails it should return an error",
			manager: &managerRecorder{
				MigrationLockError: errors.New("AcquireMigrationLock error"),
			},
			setupRegistry: func(manager *managerRecorder) {},
			expectedErr:   "failed to acquire the migration lock (AcquireMigrationLock error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 0)
			},
		},
		{
			name: "when ListStatuses fails it should return an error",
			manager: &managerRecorder{
				ListStatusesError: errors.New("ListStatuses error"),
			},
			setupRegistry: func(manager *managerRecorder) {},
			expectedErr:   "failed to list the persisted statuses (ListStatuses error)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when persisted migrations have invalid status it should return an error",
			manager: &managerRecorder{
				PersistedMigrations: []PersistedStatus{
					{Order: 1, Status: "INVALID"},
				},
			},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(standardRegisteredMigration(manager, Order(1)))
			},
			expectedErr: "the value 'INVALID' is not one of the allowed values",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when there are failed migrations it should try them again",
			manager: &managerRecorder{
				PersistedMigrations: []PersistedStatus{
					{Order: 1, Status: Failed},
				},
			},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(standardRegisteredMigration(manager, Order(1)))
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
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name:    "when registered.Migrate fails it should return an error",
			manager: &managerRecorder{},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(&Registration{
					Order: 1,
					Migrate: func(ctx context.Context) error {
						manager.Operations = append(manager.Operations, "Migration1.Migrate()")
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
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when setting the status to PENDING fails it should return an error",
			manager: &managerRecorder{
				FailOnStatus: string(Pending),
			},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(standardRegisteredMigration(manager, Order(1)))
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
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when there are persisted migrations not in the registry it return an error",
			manager: &managerRecorder{
				PersistedMigrations: []PersistedStatus{
					{Order: 1, Status: Completed},
				},
			},
			setupRegistry: func(manager *managerRecorder) {},
			expectedErr:   "found persisted migration(s) that are not in the registry",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when ReleaseMigrationLock fails it should return an error",
			manager: &managerRecorder{
				ReleaseMigrationLockError: errors.New("ReleaseMigrationLock error"),
			},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(standardRegisteredMigration(manager, Order(1)))
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
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when MigrationLockHeartbeat fails continuously it should cancel context and stop migrations",
			manager: &managerRecorder{
				MigrationLockHeartbeatError: errors.New("heartbeat error"),
			},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(&Registration{
					Order: 1,
					Migrate: func(ctx context.Context) error {
						<-ctx.Done()
						manager.Operations = append(manager.Operations, "Migration1.Migrate()")
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
					cfg, _ := config.Process[Config](config.WithPrefix(ConfigPrefix))
					cfg.HeartbeatIntervalMilliseconds = 1
					cfg.HeartbeatFailureRetryCount = 2
					return cfg, nil
				}),
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.HeartbeatCount, 3)
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when MigrationLockHeartbeat fails continuously and ReleaseMigrationLock fails it should cancel context and stop migrations",
			manager: &managerRecorder{
				MigrationLockHeartbeatError: errors.New("heartbeat error"),
				ReleaseMigrationLockError:   errors.New("ReleaseMigrationLockError error"),
			},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(&Registration{
					Order: 1,
					Migrate: func(ctx context.Context) error {
						<-ctx.Done()
						manager.Operations = append(manager.Operations, "Migration1.Migrate()")
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
					cfg, _ := config.Process[Config](config.WithPrefix(ConfigPrefix))
					cfg.HeartbeatIntervalMilliseconds = 1
					cfg.HeartbeatFailureRetryCount = 2
					return cfg, nil
				}),
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.HeartbeatCount, 3)
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when MigrationLockHeartbeat succeeds it should not prevent progress",
			manager: &managerRecorder{
				Heartbeat: make(chan struct{}),
			},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(&Registration{
					Order: 1,
					Migrate: func(ctx context.Context) error {
						<-manager.Heartbeat
						manager.Operations = append(manager.Operations, "Migration1.Migrate()")
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
					cfg, _ := config.Process[Config](config.WithPrefix(ConfigPrefix))
					cfg.HeartbeatIntervalMilliseconds = 10
					return cfg, nil
				}),
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.True(t, manager.HeartbeatCount > 0)
			},
		},
		{
			name: "when there are two migration statuses with the same order it should return an error",
			manager: &managerRecorder{
				PersistedMigrations: []PersistedStatus{
					{Order: 1, Status: Completed},
					{Order: 1, Status: Completed},
				},
			},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(standardRegisteredMigration(manager, Order(1)))
			},
			expectedErr: "found two persisted statuses with order 1",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when a migration with a completed status is encountered it should skip the migration",
			manager: &managerRecorder{
				PersistedMigrations: []PersistedStatus{
					{Order: 1, Status: Completed},
				},
			},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(standardRegisteredMigration(manager, Order(1)))
			},
			expectedErr: "",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when PersistStatus fails when setting status to STARTED it should return an error",
			manager: &managerRecorder{
				FailOnStatus: string(Started),
			},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(standardRegisteredMigration(manager, Order(1)))
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
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when PersistStatus fails when setting status to COMPLETED it should return an error",
			manager: &managerRecorder{
				FailOnStatus: string(Completed),
			},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(standardRegisteredMigration(manager, Order(1)))
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
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
		{
			name: "when a persisted status is out of sequence with the registered migrations it should return an error",
			manager: &managerRecorder{
				PersistedMigrations: []PersistedStatus{
					{Order: 2, Status: Completed},
				},
			},
			setupRegistry: func(manager *managerRecorder) {
				MustRegister(standardRegisteredMigration(manager, Order(1)))
				MustRegister(standardRegisteredMigration(manager, Order(2)))
			},
			expectedErr: "cannot run migrations out of order (found 1 but latest completed is 2)",
			expectedOps: []string{
				"AcquireDBLock()",
				"EnsureDataStores()",
				"ReleaseDBLock()",
				"AcquireMigrationLock()",
				"ListStatuses()",
			},
			asserts: func(t *testing.T, manager *managerRecorder) {
				t.Helper()
				assert.Equals(t, manager.MigrationUnlockCount, 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry.Clear()
			if tt.setupRegistry != nil {
				tt.setupRegistry(tt.manager)
			}
			err := Migrate(tt.manager, tt.options...)
			if tt.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorPart(t, err, tt.expectedErr)
			}
			assert.Equals(t, tt.expectedOps, tt.manager.Operations)
			if tt.asserts != nil {
				tt.asserts(t, tt.manager)
			}
		})
	}
}

func TestPersistStatus(t *testing.T) {
	t.Parallel()

	assert.NoError(t, validation.Struct(PersistedStatus{Order: 0, Status: Pending}))
	assert.NoError(t, validation.Struct(PersistedStatus{Order: 1, Status: Started}))
	assert.NoError(t, validation.Struct(PersistedStatus{Order: 2, Status: Failed}))
	assert.NoError(t, validation.Struct(PersistedStatus{Order: 3, Status: Completed}))

	assert.Error(t, validation.Struct(PersistedStatus{Order: 4, Status: ""}))
	assert.Error(t, validation.Struct(PersistedStatus{Order: 5, Status: "UNKNOWN"}))
	assert.Error(t, validation.Struct(PersistedStatus{Order: -1, Status: Pending}))
}
