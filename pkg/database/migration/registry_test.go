package migration

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/TriangleSide/GoBase/pkg/test/assert"
)

func TestRegistry(t *testing.T) {
	registry.Clear()

	order := atomic.Int32{}
	order.Store(0)

	t.Run("when an order is registered twice it should fail", func(t *testing.T) {
		t.Parallel()
		registrationOrder := Order(order.Add(1))
		reggy := &Registration{
			Order:   registrationOrder,
			Migrate: func(ctx context.Context) error { return nil },
			Enabled: true,
		}
		MustRegister(reggy)
		assert.PanicPart(t, func() {
			MustRegister(reggy)
		}, fmt.Sprintf("order %d already exists", registrationOrder))
	})

	t.Run("when the registration fails the validation it should panic", func(t *testing.T) {
		t.Parallel()
		registrationOrder := Order(order.Add(1))
		assert.PanicPart(t, func() {
			MustRegister(&Registration{
				Order:   registrationOrder,
				Migrate: nil,
				Enabled: true,
			})
		}, fmt.Sprintf("Validation failed for registration"))
	})

	t.Run("when orderedRegistrations is called the registration should be in order", func(t *testing.T) {
		t.Parallel()
		const count = 32
		for i := 0; i < count; i++ {
			registrationOrder := Order(order.Add(1))
			MustRegister(&Registration{
				Order:   registrationOrder,
				Migrate: func(ctx context.Context) error { return nil },
				Enabled: true,
			})
		}
		ordered := orderedRegistrations()
		for i := 1; i < len(ordered); i++ {
			assert.True(t, ordered[i-1].Order < ordered[i].Order)
		}
	})

	t.Run("when a non-Registration type is stored it should panic", func(t *testing.T) {
		t.Cleanup(func() {
			registry.Clear()
		})
		registry.Store(Order(1), "invalid registration type")
		assert.PanicPart(t, func() {
			_ = orderedRegistrations()
		}, fmt.Sprintf("order %d was not a *Registration", Order(1)))
	})

}
