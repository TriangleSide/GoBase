package structs_test

import (
	"strings"
	"testing"
	"time"

	"github.com/TriangleSide/GoBase/pkg/ptr"
	"github.com/TriangleSide/GoBase/pkg/structs"
	"github.com/TriangleSide/GoBase/pkg/test/assert"
)

type unmarshallTestStruct struct {
	Value string
}

func (t *unmarshallTestStruct) UnmarshalText(text []byte) error {
	t.Value = string(text)
	return nil
}

func TestAssign(t *testing.T) {
	t.Parallel()

	type testDeepEmbeddedStruct struct {
		DeepEmbeddedValue *string
	}

	type testEmbeddedStruct struct {
		testDeepEmbeddedStruct
		EmbeddedValue string
	}

	type testInternalStruct struct {
		Value string `json:"value"`
	}

	type testStruct struct {
		testEmbeddedStruct

		StringValue     string
		IntValue        int
		Int8Value       int8
		UintValue       uint
		Uint8Value      uint8
		Float32Value    float32
		FloatValue      float64
		BoolValue       bool
		StructValue     testInternalStruct
		MapValue        map[string]testInternalStruct
		UnmarshallValue unmarshallTestStruct
		TimeValue       time.Time

		StringPtrValue     *string
		IntPtrValue        *int
		Int8PtrValue       *int8
		UintPtrValue       *uint
		Uint8PtrValue      *uint8
		Float32PtrValue    *float32
		FloatPtrValue      *float64
		BoolPtrValue       *bool
		StructPtrValue     *testInternalStruct
		MapPtrValue        *map[string]testInternalStruct
		UnmarshallPtrValue *unmarshallTestStruct
		TimePtrValue       *time.Time

		ListStringValue []string
		ListIntValue    []int
		ListFloatValue  []float64
		ListBoolValue   []bool
		ListStructValue []testInternalStruct

		ListStringPtrValue []*string
		ListIntPtrValue    []*int
		ListFloatPtrValue  []*float64
		ListBoolPtrValue   []*bool
		ListStructPtrValue []*testInternalStruct

		UnhandledValue uintptr
	}

	t.Run("when a value is assigned on a value that is not a struct", func(t *testing.T) {
		t.Parallel()
		assert.PanicPart(t, func() {
			_ = structs.AssignToField(new(int), "StringValue", "test")
		}, "obj must be a pointer to a struct")
	})

	t.Run("when an unknown field is set it should panic", func(t *testing.T) {
		values := &testStruct{}
		assert.PanicPart(t, func() {
			_ = structs.AssignToField(values, "NonExistentField", "some value")
		}, "no field 'NonExistentField' in struct")
	})

	t.Run("it should set a time field", func(t *testing.T) {
		t.Parallel()
		const setValue = "2024-01-01T12:34:56Z"
		expectedTime, err := time.Parse(time.RFC3339, setValue)
		assert.NoError(t, err)
		values := &testStruct{}
		assert.NoError(t, structs.AssignToField(values, "TimeValue", setValue))
		assert.Equals(t, expectedTime, values.TimeValue)
		assert.NoError(t, structs.AssignToField(values, "TimePtrValue", setValue))
		assert.Equals(t, expectedTime, *values.TimePtrValue)
	})

	t.Run("when normal value assignments are done it should assign values correctly", func(t *testing.T) {
		t.Parallel()
		subTests := []struct {
			fieldName string
			strValue  string
			value     func(values *testStruct) any
			expected  interface{}
		}{
			{"EmbeddedValue", "embedded", func(ts *testStruct) any { return ts.EmbeddedValue }, "embedded"},
			{"DeepEmbeddedValue", "deepEmbedded", func(ts *testStruct) any { return *ts.DeepEmbeddedValue }, "deepEmbedded"},
			{"StringValue", "str", func(ts *testStruct) any { return ts.StringValue }, "str"},
			{"StringPtrValue", "strPtr", func(ts *testStruct) any { return *ts.StringPtrValue }, "strPtr"},
			{"IntValue", "123", func(ts *testStruct) any { return ts.IntValue }, 123},
			{"IntPtrValue", "123", func(ts *testStruct) any { return *ts.IntPtrValue }, 123},
			{"Int8Value", "-32", func(ts *testStruct) any { return ts.Int8Value }, int8(-32)},
			{"Int8PtrValue", "-32", func(ts *testStruct) any { return *ts.Int8PtrValue }, int8(-32)},
			{"UintValue", "456", func(ts *testStruct) any { return ts.UintValue }, uint(456)},
			{"UintPtrValue", "456", func(ts *testStruct) any { return *ts.UintPtrValue }, uint(456)},
			{"Uint8Value", "99", func(ts *testStruct) any { return ts.Uint8Value }, uint8(99)},
			{"Uint8PtrValue", "99", func(ts *testStruct) any { return *ts.Uint8PtrValue }, uint8(99)},
			{"Float32Value", "123.45", func(ts *testStruct) any { return ts.Float32Value }, float32(123.45)},
			{"Float32PtrValue", "123.45", func(ts *testStruct) any { return *ts.Float32PtrValue }, float32(123.45)},
			{"FloatValue", "678.90", func(ts *testStruct) any { return ts.FloatValue }, float64(678.90)},
			{"FloatPtrValue", "678.90", func(ts *testStruct) any { return *ts.FloatPtrValue }, float64(678.90)},
			{"BoolValue", "true", func(ts *testStruct) any { return ts.BoolValue }, true},
			{"BoolValue", "false", func(ts *testStruct) any { return ts.BoolValue }, false},
			{"BoolValue", "1", func(ts *testStruct) any { return ts.BoolValue }, true},
			{"BoolValue", "0", func(ts *testStruct) any { return ts.BoolValue }, false},
			{"BoolPtrValue", "true", func(ts *testStruct) any { return *ts.BoolPtrValue }, true},
			{"StructValue", `{"value":"nested"}`, func(ts *testStruct) any { return ts.StructValue }, testInternalStruct{Value: "nested"}},
			{"StructPtrValue", `{"value":"nestedPtr"}`, func(ts *testStruct) any { return *ts.StructPtrValue }, testInternalStruct{Value: "nestedPtr"}},
			{"MapValue", `{"key1":{"value":"value1"}}`, func(ts *testStruct) any { return ts.MapValue }, map[string]testInternalStruct{"key1": {Value: "value1"}}},
			{"MapPtrValue", `{"key1":{"value":"valuePtr"}}`, func(ts *testStruct) any { return *ts.MapPtrValue }, map[string]testInternalStruct{"key1": {Value: "valuePtr"}}},
			{"UnmarshallValue", "custom text", func(ts *testStruct) any { return ts.UnmarshallValue }, unmarshallTestStruct{Value: "custom text"}},
			{"UnmarshallPtrValue", "custom text ptr", func(ts *testStruct) any { return *ts.UnmarshallPtrValue }, unmarshallTestStruct{Value: "custom text ptr"}},
			{"ListStringValue", `["one", "two", "three"]`, func(ts *testStruct) any { return ts.ListStringValue }, []string{"one", "two", "three"}},
			{"ListStringPtrValue", `["one", "two", "three"]`, func(ts *testStruct) any { return ts.ListStringPtrValue }, []*string{ptr.Of("one"), ptr.Of("two"), ptr.Of("three")}},
			{"ListIntValue", `[1, 2, 3]`, func(ts *testStruct) any { return ts.ListIntValue }, []int{1, 2, 3}},
			{"ListIntPtrValue", `[1, 2, 3]`, func(ts *testStruct) any { return ts.ListIntPtrValue }, []*int{ptr.Of(1), ptr.Of(2), ptr.Of(3)}},
			{"ListFloatValue", `[1.1, 2.2, 3.3]`, func(ts *testStruct) any { return ts.ListFloatValue }, []float64{1.1, 2.2, 3.3}},
			{"ListFloatPtrValue", `[1.1, 2.2, 3.3]`, func(ts *testStruct) any { return ts.ListFloatPtrValue }, []*float64{ptr.Of(1.1), ptr.Of(2.2), ptr.Of(3.3)}},
			{"ListBoolValue", `[true, false, true]`, func(ts *testStruct) any { return ts.ListBoolValue }, []bool{true, false, true}},
			{"ListBoolPtrValue", `[true, false, true]`, func(ts *testStruct) any { return ts.ListBoolPtrValue }, []*bool{ptr.Of(true), ptr.Of(false), ptr.Of(true)}},
			{"ListStructValue", `[{"value":"nested1"}, {"value":"nested2"}]`, func(ts *testStruct) any { return ts.ListStructValue }, []testInternalStruct{{Value: "nested1"}, {Value: "nested2"}}},
			{"ListStructPtrValue", `[{"value":"nested1"}]`, func(ts *testStruct) any { return ts.ListStructPtrValue }, []*testInternalStruct{ptr.Of(testInternalStruct{Value: "nested1"})}},
		}
		for _, subTest := range subTests {
			values := &testStruct{}
			assert.NoError(t, structs.AssignToField(values, subTest.fieldName, subTest.strValue))
			assert.Equals(t, subTest.expected, subTest.value(values))
		}
	})

	t.Run("when values that can't be parsed in their fields are assgined it should return an error", func(t *testing.T) {
		t.Parallel()
		subTests := []struct {
			fieldName string
			strValue  string
			errorPart string
		}{
			{"IntValue", "not an integer", "strconv.ParseInt"},
			{"IntValue", strings.Repeat("1", 100), "strconv.ParseInt"},
			{"IntValue", "123.456", "strconv.ParseInt"},
			{"UintValue", "-123", "strconv.ParseUint"},
			{"FloatValue", "not a float", "strconv.ParseFloat"},
			{"BoolValue", "2", "strconv.ParseBool"},
			{"StructValue", "not a json object", "json unmarshal error"},
			{"MapValue", "not a json object", "json unmarshal error"},
			{"ListIntValue", `["one", "two", "three"]`, "json unmarshal error"},
			{"ListIntPtrValue", `["one", "two", "three"]`, "json unmarshal error"},
			{"ListBoolValue", `["true", "false", "maybe"]`, "json unmarshal error"},
			{"ListStructValue", `[{"value":"nested1"}, {"value":}]`, "json unmarshal error"},
			{"ListFloatValue", `["1.1", "two", "3.3"]`, "json unmarshal error"},
			{"TimeValue", "this is not a time string", "parsing time"},
			{"TimePtrValue", "this is not a time string", "parsing time"},
			{"UnhandledValue", "unhandled", "unsupported field type"},
		}
		for _, subTest := range subTests {
			values := &testStruct{}
			err := structs.AssignToField(values, subTest.fieldName, subTest.strValue)
			assert.ErrorPart(t, err, subTest.errorPart)
		}
	})
}
