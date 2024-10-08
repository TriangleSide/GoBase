package envprocessor

import (
	"fmt"
	"os"

	"github.com/TriangleSide/GoBase/pkg/utils/assign"
	"github.com/TriangleSide/GoBase/pkg/utils/fields"
	"github.com/TriangleSide/GoBase/pkg/utils/stringcase"
	"github.com/TriangleSide/GoBase/pkg/validation"
)

// EnvName is used to indicate that the value of the variable is the name of an environment variable.
type EnvName string

const (
	// FormatTag is the field name pre-processor. Is a field is called StructField and has a snake-case formatter,
	// it is transformed into STRUCT_FIELD.
	FormatTag = "config_format"

	// DefaultTag is the default to use in case there is no environment variable that matches the formatted field name.
	DefaultTag = "config_default"

	// FormatTypeSnake tells the processor to transform the field name into snake-case. StructField becomes STRUCT_FIELD.
	FormatTypeSnake = "snake"
)

// config is the configuration for the ProcessAndValidate function.
type config struct {
	prefix string
}

// Option is used to set parameters for the environment variable processor.
type Option func(*config)

// WithPrefix sets the prefix to look for in the environment variables.
// Given a struct field named Value and the prefix TEST, the processor will look for TEST_VALUE.
func WithPrefix(prefix string) Option {
	return func(p *config) {
		p.prefix = prefix
	}
}

// ProcessAndValidate fills out the fields of a struct from the environment variables.
func ProcessAndValidate[T any](opts ...Option) (*T, error) {
	cfg := &config{
		prefix: "",
	}

	for _, opt := range opts {
		opt(cfg)
	}

	fieldsMetadata := fields.StructMetadata[T]()
	conf := new(T)

	for fieldName, fieldMetadata := range fieldsMetadata.Iterator() {
		formatValue, hasFormatTag := fieldMetadata.Tags[FormatTag]
		if !hasFormatTag {
			continue
		}

		var formattedEnvName string
		switch formatValue {
		case FormatTypeSnake:
			formattedEnvName = stringcase.CamelToSnake(fieldName)
			if cfg.prefix != "" {
				formattedEnvName = fmt.Sprintf("%s_%s", cfg.prefix, formattedEnvName)
			}
		default:
			panic(fmt.Sprintf("invalid config format (%s)", formatValue))
		}

		envValue, hasEnvValue := os.LookupEnv(formattedEnvName)
		if hasEnvValue {
			if err := assign.StructField(conf, fieldName, envValue); err != nil {
				return nil, fmt.Errorf("failed to assign env var %s to field %s (%s)", envValue, fieldName, err.Error())
			}
		} else {
			defaultValue, hasDefaultTag := fieldMetadata.Tags[DefaultTag]
			if hasDefaultTag {
				if err := assign.StructField(conf, fieldName, defaultValue); err != nil {
					return nil, fmt.Errorf("failed to assign default value %s to field %s (%s)", defaultValue, fieldName, err.Error())
				}
			}
		}
	}

	if err := validation.Struct(conf); err != nil {
		return nil, fmt.Errorf("failed while validating the configuration (%s)", err.Error())
	}

	return conf, nil
}
