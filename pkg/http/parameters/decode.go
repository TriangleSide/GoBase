package parameters

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/TriangleSide/GoBase/pkg/http/headers"
	"github.com/TriangleSide/GoBase/pkg/logger"
	reflectutils "github.com/TriangleSide/GoBase/pkg/utils/reflect"
	"github.com/TriangleSide/GoBase/pkg/validation"
)

// Decode populates a parameter struct with values from an HTTP request and performs validation on the struct.
func Decode[T any](request *http.Request) (*T, error) {
	params := new(T)
	if reflect.ValueOf(*params).Kind() != reflect.Struct {
		panic("the generic must be a struct")
	}

	tagToLookupKeyToFieldName, err := ExtractAndValidateFieldTagLookupKeys[T]()
	if err != nil {
		panic(fmt.Sprintf("tags are not correctly formatted (%s)", err.Error()))
	}

	if err := decodeJSONBodyParameters(params, request); err != nil {
		return nil, fmt.Errorf("failed to parse json body parameters (%s)", err.Error())
	}

	if err := decodeQueryParameters(params, tagToLookupKeyToFieldName, request); err != nil {
		return nil, fmt.Errorf("failed to parse query parameters (%s)", err.Error())
	}

	if err := decodeHeaderParameters(params, tagToLookupKeyToFieldName, request); err != nil {
		return nil, fmt.Errorf("failed to parse header parameters (%s)", err.Error())
	}

	if err := decodePathParameters(params, tagToLookupKeyToFieldName, request); err != nil {
		return nil, fmt.Errorf("failed to parse path parameters (%s)", err.Error())
	}

	if err := validation.Struct(params); err != nil {
		return nil, fmt.Errorf("validation failed for request parameters (%s)", err.Error())
	}

	return params, nil
}

// decodeJSONBodyParameters decodes JSON from the request body into the parameter struct.
func decodeJSONBodyParameters[T any](params *T, request *http.Request) error {
	if strings.EqualFold(request.Header.Get(headers.ContentType), headers.ContentTypeApplicationJson) {
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()
		defer func() {
			if err := request.Body.Close(); err != nil {
				logger.LogEntry(request.Context()).WithError(err).Error("Failed to close the request body.")
			}
		}()
		if err := decoder.Decode(&params); err != nil {
			return fmt.Errorf("failed to decode json body (%s)", err.Error())
		}
	}
	return nil
}

// decodeQueryParameters identifies fields tagged with QueryTag and maps corresponding URL query parameters to these fields.
func decodeQueryParameters[T any](params *T, tagToLookupKeyToFieldName TagToLookupKeyToFieldName, request *http.Request) error {
	lookupKeyToFieldName := tagToLookupKeyToFieldName[QueryTag]
	normalizer := tagToLookupKeyNormalizer[QueryTag]

	for queryParameterName, queryParameterValues := range request.URL.Query() {
		normalizedQueryParameterName := normalizer(queryParameterName)
		matchedFieldName, hasMatchedFieldName := lookupKeyToFieldName[normalizedQueryParameterName]
		if !hasMatchedFieldName {
			continue
		}
		if len(queryParameterValues) != 1 {
			return fmt.Errorf("expecting one value for query parameter %s but found %v", queryParameterName, queryParameterValues)
		}
		if err := reflectutils.AssignToField(params, matchedFieldName, queryParameterValues[0]); err != nil {
			return fmt.Errorf("failed to set value for query parameter %s with values of %v (%s)", queryParameterName, queryParameterValues, err.Error())
		}
	}

	return nil
}

// decodeHeaderParameters identifies fields tagged with HeaderTag and maps corresponding HTTP headers to these fields.
func decodeHeaderParameters[T any](params *T, tagToLookupKeyToFieldName TagToLookupKeyToFieldName, request *http.Request) error {
	lookupKeyToFieldName := tagToLookupKeyToFieldName[HeaderTag]
	normalizer := tagToLookupKeyNormalizer[HeaderTag]

	for headerName, headerValues := range request.Header {
		normalizedHeaderName := normalizer(headerName)
		matchedFieldName, hasMatchedFieldName := lookupKeyToFieldName[normalizedHeaderName]
		if !hasMatchedFieldName {
			continue
		}
		if len(headerValues) != 1 {
			return fmt.Errorf("expecting one value for header parameter %s but found %v", headerName, headerValues)
		}
		if err := reflectutils.AssignToField(params, matchedFieldName, headerValues[0]); err != nil {
			return fmt.Errorf("failed to set value for header parameter %s with values of %v (%s)", headerName, headerValues, err.Error())
		}
	}

	return nil
}

// decodePathParameters identifies fields tagged with PathTag and maps corresponding URL path parameters to these fields.
func decodePathParameters[T any](params *T, tagToLookupKeyToFieldName TagToLookupKeyToFieldName, request *http.Request) error {
	lookupKeyToFieldName := tagToLookupKeyToFieldName[PathTag]
	normalizer := tagToLookupKeyNormalizer[PathTag]

	for pathName, field := range lookupKeyToFieldName {
		normalizedPathName := normalizer(pathName)
		pathValue := request.PathValue(normalizedPathName)
		if pathValue == "" {
			continue
		}
		if err := reflectutils.AssignToField(params, field, pathValue); err != nil {
			return fmt.Errorf("failed to set value for path parameter %s with values of %v (%s)", pathName, pathValue, err.Error())
		}
	}

	return nil
}
