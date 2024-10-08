package fields

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"time"

	"github.com/TriangleSide/GoBase/pkg/datastructures/cache"
	"github.com/TriangleSide/GoBase/pkg/datastructures/readonlymap"
)

var (
	// tagMatchRegex matches all tag entries on a struct field.
	tagMatchRegex = regexp.MustCompile(`(\w+):"([^"]*)"`)

	// typeToMetadataCache is used to cache the result of the StructMetadata function.
	typeToMetadataCache = cache.New[reflect.Type, *readonlymap.ReadOnlyMap[string, *FieldMetadata]]()
)

// FieldMetadata is the metadata extracted from struct fields.
type FieldMetadata struct {
	Type      reflect.Type
	Tags      map[string]string
	Anonymous []string
}

// StructMetadata returns a map of a structs field names to their respective metadata.
func StructMetadata[T any]() *readonlymap.ReadOnlyMap[string, *FieldMetadata] {
	reflectType := reflect.TypeOf(*new(T))
	fieldsToMetadata, _ := typeToMetadataCache.GetOrSet(reflectType, func(reflectType reflect.Type) (*readonlymap.ReadOnlyMap[string, *FieldMetadata], *time.Duration, error) {
		fieldsToMetadata := make(map[string]*FieldMetadata)
		processType(reflectType, fieldsToMetadata, make([]string, 0))
		readOnlyMap := readonlymap.NewBuilder[string, *FieldMetadata]().SetMap(fieldsToMetadata).Build()
		return readOnlyMap, nil, nil
	})
	return fieldsToMetadata
}

// processType takes a struct type, lists all of its fields, and builds the metadata for it.
// If the struct contains an embedded anonymous struct, it appends its name to the anonymous name chain.
// If a field name is not unique, a panic occurs. This includes field names of the anonymous structs.
func processType(reflectType reflect.Type, fieldsToMetadata map[string]*FieldMetadata, anonymousChain []string) {
	if reflectType.Kind() == reflect.Ptr {
		reflectType = reflectType.Elem()
	}

	if reflectType.Kind() != reflect.Struct {
		panic("Type must be a struct or a pointer to a struct.")
	}

	for fieldIndex := 0; fieldIndex < reflectType.NumField(); fieldIndex++ {
		field := reflectType.Field(fieldIndex)

		anonymousChainCopy := make([]string, len(anonymousChain))
		copy(anonymousChainCopy, anonymousChain)

		if field.Anonymous {
			anonymousChainCopy = append(anonymousChainCopy, field.Name)
			processType(field.Type, fieldsToMetadata, anonymousChainCopy)
			continue
		}

		if _, alreadyHasFieldName := fieldsToMetadata[field.Name]; alreadyHasFieldName {
			panic(fmt.Sprintf("field %s is ambiguous", field.Name))
		}

		metadata := &FieldMetadata{}
		metadata.Type = field.Type
		metadata.Tags = make(map[string]string)
		metadata.Anonymous = anonymousChainCopy

		if len(string(field.Tag)) != 0 {
			matches := tagMatchRegex.FindAllStringSubmatch(string(field.Tag), -1)
			for _, match := range matches {
				tagKey := match[1]
				tagValue := match[2]
				metadata.Tags[tagKey] = tagValue
			}
		}

		fieldsToMetadata[field.Name] = metadata
	}
}

// StructValueFromName returns the fields value if it exists.
func StructValueFromName[T any](structInstance T, fieldName string) (reflect.Value, error) {
	v := reflect.ValueOf(structInstance)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return reflect.Value{}, errors.New("struct instance cannot be nil")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, errors.New("type must be a struct or a pointer to a struct")
	}
	field := v.FieldByName(fieldName)
	if !field.IsValid() {
		return reflect.Value{}, fmt.Errorf("field %s does not exist in the struct", fieldName)
	}
	return field, nil
}
