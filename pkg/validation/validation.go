package validation

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/TriangleSide/GoBase/pkg/utils/fields"
)

const (
	// ValidatorsSep is the separator between validation names. For example: "required,oneof=THIS THAT".
	ValidatorsSep = ","

	// NameAndInstructionsSep is the separator between the validation name and the instructions.
	// For example: "oneof=THIS THAT".
	NameAndInstructionsSep = "="

	// Tag is the name of the struct field tag.
	//
	// type Example struct {
	//     Value int `validate:"gt=0"`
	// }
	Tag = "validate"
)

var (
	// registeredValidation stores the validation name and the callback to verify the value.
	registeredValidation = sync.Map{}
)

// CallbackParameters are the parameters sent to the validation callback.
// Struct fields are only set on Struct validation.
type CallbackParameters struct {
	StructValidation bool
	StructValue      reflect.Value
	StructFieldName  string
	Value            reflect.Value
	Parameters       string
}

// Callback checks a value against the instructions for the validator.
type Callback func(*CallbackParameters) error

// MustRegisterValidator sets the callback for a validator.
func MustRegisterValidator(name Validator, callback Callback) {
	_, alreadyExists := registeredValidation.LoadOrStore(string(name), callback)
	if alreadyExists {
		panic(fmt.Sprintf("Validation named %s already exists.", name))
	}
}

// splitValidatorNameAndInstruction takes a `validator=instructions` string and splits it.
func splitValidatorNameAndInstruction(nameToInstruction string) (string, string, error) {
	const maxNameToInstructionParts = 2
	const validatorNameIndex = 0
	const validatorInstructionsIndex = 1
	nameToInstructionParts := strings.Split(nameToInstruction, NameAndInstructionsSep)
	if len(nameToInstructionParts) > maxNameToInstructionParts {
		return "", "", fmt.Errorf("malformed validator and instruction")
	}
	validatorName := nameToInstructionParts[validatorNameIndex]
	validatorInstructions := ""
	if len(nameToInstructionParts) >= (validatorInstructionsIndex + 1) {
		validatorInstructions = nameToInstructionParts[validatorInstructionsIndex]
	}
	return validatorName, validatorInstructions, nil
}

// checkValidatorsAgainstValue validates a value based on the provided validation tag.
// Is returns validation Violations if found, or an error if anything went wrong while validating.
func checkValidatorsAgainstValue(structValidation bool, structValue reflect.Value, structFieldName string, fieldValue reflect.Value, validationTagContents string) (*Violations, error) {
	violations := NewViolations()

	if strings.TrimSpace(validationTagContents) == "" {
		return nil, fmt.Errorf("%s tag cannot be empty", Tag)
	}
	namesToInstructions := strings.Split(validationTagContents, ValidatorsSep)

	for i := 0; i < len(namesToInstructions); i++ {
		validatorName, validatorInstructions, err := splitValidatorNameAndInstruction(namesToInstructions[i])
		if err != nil {
			return nil, err
		}

		callbackNotCast, callbackFound := registeredValidation.Load(validatorName)
		if !callbackFound {
			return nil, fmt.Errorf("validation with name '%s' is not registered", validatorName)
		}
		callback := callbackNotCast.(Callback)
		callbackParameters := &CallbackParameters{
			StructValidation: structValidation,
			StructValue:      structValue,
			StructFieldName:  structFieldName,
			Value:            fieldValue,
			Parameters:       validatorInstructions,
		}

		if err := callback(callbackParameters); err != nil {
			var violation *Violation
			if errors.As(err, &violation) {
				violations.AddViolation(violation)
				continue
			}

			var validatorsStop *stopValidators
			if errors.As(err, &validatorsStop) {
				return violations, nil
			}

			var valuesNew *newValues
			if errors.As(err, &valuesNew) {
				restOfValidationTag := strings.Join(namesToInstructions[i+1:], ValidatorsSep)
				if restOfValidationTag == "" {
					return violations, nil
				}
				for _, newValue := range valuesNew.values {
					newValueViolations, newValueValidationError := checkValidatorsAgainstValue(structValidation, structValue, structFieldName, newValue, restOfValidationTag)
					if newValueValidationError != nil {
						return nil, newValueValidationError
					}
					violations.AddViolations(newValueViolations)
				}
				return violations, nil
			}

			return nil, err
		}
	}

	return violations, nil
}

// validateRecursively check if a value is a struct and runs the validation on it.
func validateRecursively(val reflect.Value) (*Violations, error) {
	violations := NewViolations()
	if ValueIsNil(val) {
		return violations, nil
	}
	DereferenceValue(&val)

	switch val.Kind() {
	case reflect.Struct:
		if err := Struct(val.Interface()); err != nil {
			var structViolations *Violations
			if errors.As(err, &structViolations) {
				violations.AddViolations(structViolations)
			} else {
				return nil, err
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			recursiveViolations, err := validateRecursively(val.Index(i))
			if err != nil {
				return nil, err
			}
			violations.AddViolations(recursiveViolations)
		}
	default:
		// This type is not handled. Do nothing.
	}

	return violations, nil
}

// Struct validates all struct fields using their validation tags, returning an error if any fail.
func Struct[T any](val T) error {
	reflectVal := reflect.ValueOf(val)
	if ValueIsNil(reflectVal) {
		return errors.New("nil parameter on struct validation")
	}
	DereferenceValue(&reflectVal)
	if reflectVal.Kind() != reflect.Struct {
		panic(fmt.Sprintf("Struct validation parameter must be a struct but got %s.", reflectVal.Kind()))
	}

	violations := NewViolations()
	structMetadataMap := fields.StructMetadataFromType(reflectVal.Type())

	for fieldName, fieldMetadata := range structMetadataMap.All() {
		fieldValueFromStruct, _ := fields.StructValueFromName(val, fieldName)

		// Check the fields validation instructions.
		if validationTag, hasValidationTag := fieldMetadata.Tags().Fetch(Tag); hasValidationTag {
			if fieldViolations, err := checkValidatorsAgainstValue(true, reflectVal, fieldName, fieldValueFromStruct, validationTag); err == nil {
				violations.AddViolations(fieldViolations)
			} else {
				return err
			}
		}

		// validate recursively if possible.
		if structFieldViolations, err := validateRecursively(fieldValueFromStruct); err == nil {
			violations.AddViolations(structFieldViolations)
		} else {
			return err
		}
	}

	return violations.NilIfEmpty()
}

// Var validates a single variable with the given instructions, returning an error if it fails.
func Var[T any](val T, validatorInstructions string) error {
	valueToVerify := reflect.ValueOf(val)
	violations, err := checkValidatorsAgainstValue(false, reflect.Value{}, "", valueToVerify, validatorInstructions)
	if err != nil {
		return err
	}
	return violations.NilIfEmpty()
}
