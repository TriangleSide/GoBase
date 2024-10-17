package validation

import (
	"errors"
	"reflect"
	"strings"
)

// Violation represents a failure for a specific validator.
type Violation struct {
	validator  Validator
	parameters *CallbackParameters
	err        error
}

// NewViolation instantiates a *Violation.
func NewViolation(name Validator, params *CallbackParameters, msg string) *Violation {
	sb := strings.Builder{}
	sb.WriteString("validation failed")
	if params.StructValidation {
		sb.WriteString(" on field '")
		sb.WriteString(params.StructFieldName)
		sb.WriteString("'")
	}
	sb.WriteString(" with validator '")
	sb.WriteString(string(name))
	sb.WriteString("'")
	if params.Parameters != "" {
		sb.WriteString(" and parameters '")
		sb.WriteString(params.Parameters)
		sb.WriteString("'")
	}
	sb.WriteString(" because ")
	sb.WriteString(msg)
	return &Violation{
		validator:  name,
		parameters: params,
		err:        errors.New(sb.String()),
	}
}

// Error ensures Violation has the error interface.
func (v *Violation) Error() string {
	return v.err.Error()
}

// Violations represents a list of violations.
type Violations struct {
	violations []*Violation
}

// NewViolations instantiates a *Violations struct.
func NewViolations() *Violations {
	return &Violations{
		violations: make([]*Violation, 0, 1),
	}
}

// AddViolations appends other violations.
func (v *Violations) AddViolations(others *Violations) {
	if others != nil && len(others.violations) > 0 {
		v.violations = append(v.violations, others.violations...)
	}
}

// AddViolation appends another violation to this list of violations.
func (v *Violations) AddViolation(other *Violation) {
	if other != nil {
		v.violations = append(v.violations, other)
	}
}

// NilIfEmpty returns nil if the violation list is empty.
func (v *Violations) NilIfEmpty() error {
	if len(v.violations) == 0 {
		return nil
	}
	return v
}

// Error ensures Violations has the error interface.
func (v *Violations) Error() string {
	errorStrings := make([]string, 0, len(v.violations))
	for _, violation := range v.violations {
		errorStrings = append(errorStrings, violation.Error())
	}
	return strings.Join(errorStrings, "; ")
}

// stopValidators is an error used to signal skipping the rest of the validators.
type stopValidators struct{}

// Error ensures stopValidators has the error interface.
func (v *stopValidators) Error() string {
	return "validation halted by stopValidators signal"
}

// newValues is an error used to signal to run the rest of the validators on these new values.
type newValues struct {
	values []reflect.Value
}

// Error ensures newValues has the error interface.
func (v *newValues) Error() string {
	return "validation continues with new values"
}
