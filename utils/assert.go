package utils

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/spf13/viper"
)

func Assert(cond bool, msg string) {
	ignoreAsserts := viper.GetBool("ignore-asserts")
	if !ignoreAsserts && !cond {
		panic(msg)
	}
}

// func Assert(expected, got interface{}, field string) {
// 	if !reflect.DeepEqual(expected, got) {
// 		slog.Error("Expected %s to be [%+v (%T)], got: [%+v (%T)]", field, expected, expected, got, got)
// 	}
// }

func isEmpty(obj interface{}) bool {
	if obj == nil {
		return true
	}

	value := reflect.ValueOf(obj)
	if value.IsNil() {
		return true
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Map, reflect.Slice:
		return value.Len() == 0
	default:
		return false
	}
}

func Empty(t *testing.T, obj interface{}) {
	if !isEmpty(obj) {
		t.Errorf("%v is not empty", obj)
	}
}

func getLength(obj interface{}) (ok bool, length int) {
	value := reflect.ValueOf(obj)
	defer func() {
		if e := recover(); e != nil {
			ok = false
		}
	}()
	return true, value.Len()
}

func Len(t *testing.T, obj interface{}, size int) {
	ok, length := getLength(obj)
	if !ok || length != size {
		t.Errorf("%v doesn't have size %v", obj, size)
	}
}

func isObjectEqual(a, b interface{}) bool {
	if a == nil || b == nil {
		return a == b
	}

	binaryA, ok := a.([]byte)
	if !ok {
		return reflect.DeepEqual(a, b)
	}

	binaryB, ok := b.([]byte)
	if !ok {
		return false
	}

	if binaryA == nil || binaryB == nil {
		return binaryA == nil && binaryB == nil
	}

	return bytes.Equal(binaryA, binaryB)
}

func isEqual(a, b interface{}) bool {
	if isObjectEqual(a, b) {
		return true
	}

	typeB := reflect.TypeOf(b)
	if typeB == nil {
		return false
	}

	valueA := reflect.ValueOf(a)
	if valueA.IsValid() && valueA.Type().ConvertibleTo(typeB) {
		return reflect.DeepEqual(valueA.Convert(typeB).Interface(), b)
	}

	return false
}

func Equal(t *testing.T, a, b interface{}) {
	if !isEqual(a, b) {
		t.Errorf("%v is not equal to %v", a, b)
	}
}

func NotEqual(t *testing.T, a, b interface{}) {
	if isEqual(a, b) {
		t.Errorf("%v is equal to %v", a, b)
	}
}

func isGreater(a, b interface{}) bool {
	a64, okA := a.(uint64)
	b64, okB := b.(uint64)
	return okA && okB && (a64 > b64)
}

func Greater(t *testing.T, a, b interface{}) {
	if !isGreater(a, b) {
		t.Errorf("%v is not greater than %v", a, b)
	}
}

func True(t *testing.T, condition bool) {
	if !condition {
		t.Errorf("condition is not true")
	}
}

// from other source

/*
package assert

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"runtime/debug"
)

// TODO using slog for logging
type AssertData interface {
    Dump() string
}
type AssertFlush interface {
    Flush()
}

var flushes []AssertFlush = []AssertFlush{}
var assertData map[string]AssertData = map[string]AssertData{}
var writer io.Writer

func AddAssertData(key string, value AssertData) {
	assertData[key] = value
}

func RemoveAssertData(key string) {
	delete(assertData, key)
}

func AddAssertFlush(flusher AssertFlush) {
    flushes = append(flushes, flusher)
}

func ToWriter(w io.Writer) {
	writer = w
}

func runAssert(msg string, args ...interface{}) {
    // There is a bit of a issue here.  if you flush you cannot assert
    // cannot be reentrant
    // TODO I am positive i could create some sort of latching that prevents the
    // reentrant problem
    for _, f := range flushes {
        f.Flush()
    }

    slogValues := []interface{}{
        "msg",
        msg,
        "area",
        "Assert",
    }
    slogValues = append(slogValues, args...)
    fmt.Fprintf(os.Stderr, "ARGS: %+v\n", args)

	for k, v := range assertData {
        slogValues = append(slogValues, k, v.Dump())
	}

    fmt.Fprintf(os.Stderr, "ASSERT\n")
    for i := 0; i < len(slogValues); i += 2 {
        fmt.Fprintf(os.Stderr, "   %s=%v\n", slogValues[i], slogValues[i + 1])
    }
    fmt.Fprintln(os.Stderr, string(debug.Stack()))
    os.Exit(1)
}

// TODO Think about passing around a context for debugging purposes
func Assert(truth bool, msg string, data ...any) {
	if !truth {
		runAssert(msg, data...)
	}
}

func Nil(item any, msg string, data ...any) {
    slog.Info("Nil Check", "item", item)
	if item == nil {
        return
    }

    slog.Error("Nil#not nil encountered")
    runAssert(msg, data...)
}

func NotNil(item any, msg string, data ...any) {
	if item == nil || reflect.ValueOf(item).Kind() == reflect.Ptr && reflect.ValueOf(item).IsNil() {
		slog.Error("NotNil#nil encountered")
		runAssert(msg, data...)
	}
}

func Never(msg string, data ...any) {
    runAssert(msg, data...)
}

func NoError(err error, msg string, data ...any) {
	if err != nil {
        data = append(data, "error", err)
		runAssert(msg, data...)
	}
}



*/
