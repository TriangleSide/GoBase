package migration

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/TriangleSide/GoBase/pkg/validation"
)

// Order represents the sequence in which migrations are meant to be run.
type Order int

// Registration defines a migration callback and its order.
type Registration struct {
	// Order is compared to other migrations to determine its run sequence.
	Order Order `validate:"gte=0"`

	// Migrate is invoked to run the migration.
	// This function MUST be idempotent and retryable.
	Migrate func(context.Context) error `validate:"required"`

	// Enabled indicates if this migration is to be run or not.
	// A migration could be disabled if another migration covers it.
	Enabled bool
}

var (
	// registry is a map of Order to *Registration.
	registry = sync.Map{}
)

// MustRegister stores a migration registration in the registry.
func MustRegister(registration *Registration) {
	if err := validation.Struct(registration); err != nil {
		panic(fmt.Sprintf("Validation failed for registration (%s).", err.Error()))
	}
	_, alreadyRegistered := registry.LoadOrStore(registration.Order, registration)
	if alreadyRegistered {
		panic(fmt.Sprintf("Registration with order %d already exists.", registration.Order))
	}
}

// orderedRegistrations returns an ordered list of the registrations in the registry.
// The registrations are sorted by their Order.
func orderedRegistrations() []*Registration {
	ordered := make([]*Registration, 0)

	registry.Range(func(key, value interface{}) bool {
		registration, castOk := value.(*Registration)
		if !castOk {
			panic(fmt.Sprintf("Registration with order %d was not a *Registration type.", key))
		}
		ordered = append(ordered, registration)
		return true
	})

	sort.Slice(ordered, func(a, b int) bool {
		return ordered[a].Order < ordered[b].Order
	})

	return ordered
}
