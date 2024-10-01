package envprocessor_test

import (
	"os"
	"testing"

	"github.com/TriangleSide/GoBase/pkg/config/envprocessor"
	"github.com/TriangleSide/GoBase/pkg/test/assert"
)

func TestEnvProcessor(t *testing.T) {
	setEnv := func(t *testing.T, envName string, value string) {
		t.Helper()
		err := os.Setenv(envName, value)
		assert.NoError(t, err)
	}

	unsetEnv := func(t *testing.T, envName string) {
		t.Helper()
		err := os.Unsetenv(envName)
		assert.NoError(t, err)
	}

	t.Run("when config_format is an invalid value", func(t *testing.T) {
		assert.PanicPart(t, func() {
			type testStruct struct {
				Value int `config_format:"not_valid"`
			}
			_, _ = envprocessor.ProcessAndValidate[testStruct]()
		}, "invalid config format")
	})

	t.Run("when the default value cannot be assigned to the struct field", func(t *testing.T) {
		type testStruct struct {
			Value *int `config_format:"snake" config_default:"NOT_AN_INT"`
		}
		conf, err := envprocessor.ProcessAndValidate[testStruct]()
		assert.ErrorPart(t, err, "failed to assign default value NOT_AN_INT to field Value")
		assert.Nil(t, conf)
	})

	t.Run("when a struct has an int field called Value with a default of 1, a validation rule of gte=0, and is required", func(t *testing.T) {
		const (
			EnvName      = "VALUE"
			DefaultValue = 1
		)

		type testStruct struct {
			Value int `config_format:"snake" config_default:"1" validate:"required,gte=0"`
		}

		t.Run("when the environment variable VALUE is set to NOT_AN_INT", func(t *testing.T) {
			t.Cleanup(func() {
				unsetEnv(t, EnvName)
			})
			setEnv(t, EnvName, "NOT_AN_INT")
			conf, err := envprocessor.ProcessAndValidate[testStruct]()
			assert.ErrorPart(t, err, "failed to assign env var NOT_AN_INT to field Value")
			assert.Nil(t, conf)
		})

		t.Run("when the environment variable VALUE is set to 2", func(t *testing.T) {
			t.Cleanup(func() {
				unsetEnv(t, EnvName)
			})
			setEnv(t, EnvName, "2")

			t.Run("it should be set in the Value field of the struct", func(t *testing.T) {
				conf, err := envprocessor.ProcessAndValidate[testStruct]()
				assert.NoError(t, err)
				assert.NotNil(t, conf)
				assert.Equals(t, conf.Value, 2)
			})

			t.Run("it should use the default if a prefix is used", func(t *testing.T) {
				conf, err := envprocessor.ProcessAndValidate[testStruct](envprocessor.WithPrefix("PREFIX"))
				assert.NoError(t, err)
				assert.NotNil(t, conf)
				assert.Equals(t, conf.Value, DefaultValue)
			})

			t.Run("when an environment variable called TEST_VALUE is set with a value of 3 it should able to be set with a prefix", func(t *testing.T) {
				const EnvNameWithPrefix = "TEST_VALUE"
				t.Cleanup(func() {
					unsetEnv(t, EnvNameWithPrefix)
				})
				setEnv(t, EnvNameWithPrefix, "3")
				conf, err := envprocessor.ProcessAndValidate[testStruct](envprocessor.WithPrefix("TEST"))
				assert.NoError(t, err)
				assert.NotNil(t, conf)
				assert.Equals(t, conf.Value, 3)
			})
		})

		t.Run("when the validation rule fails it should fail to process", func(t *testing.T) {
			t.Cleanup(func() {
				unsetEnv(t, EnvName)
			})
			setEnv(t, EnvName, "-1")
			conf, err := envprocessor.ProcessAndValidate[testStruct]()
			assert.ErrorPart(t, err, "validation failed")
			assert.Nil(t, conf)
		})

		t.Run("when no environment variable is set it should use the default value", func(t *testing.T) {
			conf, err := envprocessor.ProcessAndValidate[testStruct]()
			assert.NoError(t, err)
			assert.NotNil(t, conf)
			assert.Equals(t, conf.Value, DefaultValue)
		})
	})

	t.Run("when a struct has a field called Value with no default, validation, or required tag it should return a struct with unmodified fields", func(t *testing.T) {
		type testStruct struct {
			Value *int
		}
		conf, err := envprocessor.ProcessAndValidate[testStruct]()
		assert.NoError(t, err)
		assert.NotNil(t, conf)
		assert.Nil(t, conf.Value)
	})

	t.Run("when a struct has a field called Value with no config tags, but it has a required validation it should return and error", func(t *testing.T) {
		type testStruct struct {
			Value *int `validate:"required"`
		}
		conf, err := envprocessor.ProcessAndValidate[testStruct]()
		assert.ErrorPart(t, err, "validation failed")
		assert.Nil(t, conf)
	})

	t.Run("when a struct has a field and has an embedded anonymous struct with a field it should be able to set both fields", func(t *testing.T) {
		type embeddedStruct struct {
			EmbeddedField string `config_format:"snake" validate:"required"`
		}

		type testStruct struct {
			embeddedStruct
			Field string `config_format:"snake" validate:"required"`
		}

		const (
			EmbeddedEnvName = "EMBEDDED_FIELD"
			EmbeddedValue   = "embeddedField"
			FieldEnvName    = "FIELD"
			FieldValue      = "field"
		)

		t.Cleanup(func() {
			unsetEnv(t, EmbeddedEnvName)
			unsetEnv(t, FieldEnvName)
		})
		setEnv(t, EmbeddedEnvName, EmbeddedValue)
		setEnv(t, FieldEnvName, FieldValue)

		conf, err := envprocessor.ProcessAndValidate[testStruct]()
		assert.NoError(t, err)
		assert.NotNil(t, conf)
		assert.Equals(t, conf.EmbeddedField, EmbeddedValue)
		assert.Equals(t, conf.Field, FieldValue)
	})
}
