package dspviz

import (
	"math"
	"reflect"
)

// Every document this package emits -- the server's responses, SignalStats,
// SignalReport -- is sanitized and then marshaled from its own tagged struct.
//
// Sanitizing first is not optional. A decibel scale produces infinities, and
// encoding/json refuses one rather than leaving a gap; the values are mapped
// onto the edge of the range instead.

// sanitize returns a deep copy of v with every non-finite float replaced.
// Unexported struct fields are left at their zero value, since encoding/json
// does not read them either.
func sanitize(v any) any {
	if v == nil {
		return nil
	}
	return sanitizeValue(reflect.ValueOf(v)).Interface()
}

func sanitizeValue(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		out := reflect.New(v.Type()).Elem()
		out.SetFloat(cleanFloat(v.Float()))
		return out

	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(sanitizeValue(v.Elem()))
		return out

	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(sanitizeValue(v.Elem()))
		return out

	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		t := v.Type()
		for i := range v.NumField() {
			if !t.Field(i).IsExported() {
				continue
			}
			out.Field(i).Set(sanitizeValue(v.Field(i)))
		}
		return out

	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		for iter := v.MapRange(); iter.Next(); {
			out.SetMapIndex(iter.Key(), sanitizeValue(iter.Value()))
		}
		return out

	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := range v.Len() {
			out.Index(i).Set(sanitizeValue(v.Index(i)))
		}
		return out

	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := range v.Len() {
			out.Index(i).Set(sanitizeValue(v.Index(i)))
		}
		return out
	}
	return v
}

// cleanFloat maps the unrepresentable onto the edge of the plot rather than to
// zero, which would draw a spike where there is silence.
func cleanFloat(x float64) float64 {
	switch {
	case math.IsNaN(x):
		return 0
	case math.IsInf(x, -1):
		return -1e300
	case math.IsInf(x, 1):
		return 1e300
	}
	return x
}
