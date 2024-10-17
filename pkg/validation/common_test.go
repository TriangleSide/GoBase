package validation

import (
	"github.com/TriangleSide/GoBase/pkg/test/assert"
	"reflect"
	"testing"
)

func TestCommonFunctions(t *testing.T) {
	t.Parallel()

	t.Run("when nil is passed to DereferenceValue it should do nothing", func(t *testing.T) {
		t.Parallel()
		invalidValue := reflect.ValueOf(nil)
		DereferenceValue(&invalidValue)
		assert.Equals(t, invalidValue, reflect.ValueOf(nil))
	})
}
