package main

import (
	"encoding/json"
	"math"
	"testing"
)

// TestJSONValue exercises jsonValue's round trip for each SQLite value
// kind, per spec §7's value table -- the same conversion Phase 4's stdio
// protocol will reuse (value.go's doc comment).
func TestJSONValue(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"null", nil, "null"},
		{"integer", int64(42), "42"},
		{"integer negative", int64(-7), "-7"},
		{"real with fraction", float64(88.5), "88.5"},
		{"real whole number gets a decimal point", float64(88), "88.0"},
		{"real zero", float64(0), "0.0"},
		{"text", "alice", `"alice"`},
		{"text empty", "", `""`},
		{"blob", []byte{0x89, 0x50, 0x4e, 0x47}, `["iVBORw=="]`},
		{"blob empty", []byte{}, `[""]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			jv, err := jsonValue(c.in)
			if err != nil {
				t.Fatalf("jsonValue(%#v): %v", c.in, err)
			}
			b, err := json.Marshal(jv)
			if err != nil {
				t.Fatalf("json.Marshal(%#v): %v", jv, err)
			}
			if got := string(b); got != c.want {
				t.Fatalf("jsonValue(%#v) marshaled = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestJSONValueRejectsUnexpectedType(t *testing.T) {
	if _, err := jsonValue(true); err == nil {
		t.Fatal("expected an error for a value type database/sql never actually returns")
	}
}

func TestJSONRealSpecialValues(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want string
	}{
		{"+Inf", math.Inf(1), "9e999"},
		{"-Inf", math.Inf(-1), "-9e999"},
		{"NaN", math.NaN(), "null"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := string(jsonReal(c.in)); got != c.want {
				t.Fatalf("jsonReal(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestJSONValueBLOBRoundTripsThroughBase64 confirms the exact encoding
// (standard alphabet, padded) spec §7 requires, not just that some
// base64 variant was used.
func TestJSONValueBLOBRoundTripsThroughBase64(t *testing.T) {
	blob := []byte("hello, world! this needs padding")
	jv, err := jsonValue(blob)
	if err != nil {
		t.Fatal(err)
	}
	arr, ok := jv.([1]string)
	if !ok {
		t.Fatalf("jsonValue(BLOB) = %#v, want a [1]string", jv)
	}
	if len(arr[0])%4 != 0 {
		t.Fatalf("base64 output %q is not standard-padded", arr[0])
	}
}
