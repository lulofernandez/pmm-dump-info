// Package qantype checks the values pmm-dump writes for the ClickHouse
// column types of pmm.metrics. It is shared by the tool and by the schema
// generator (tools/qanschema), which refuses a type the tool cannot check.
package qantype

import (
	"regexp"
	"strconv"
	"strings"
)

// pmm-dump writes each value with fmt %v of what clickhouse-go scans:
// DateTime is a time.Time, arrays are Go slices, enums their name.
var GoTime = regexp.MustCompile(`^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d [+-]\d{4} \S+$`)

var enumName = regexp.MustCompile(`'((?:[^'\\]|\\.)*)'=`)

// Check returns the check for the values of a ClickHouse column type, false
// when the type is not supported.
func Check(t string) (func(string) bool, bool) {
	inner := func(prefix string) (string, bool) {
		if strings.HasPrefix(t, prefix+"(") && strings.HasSuffix(t, ")") {
			return t[len(prefix)+1 : len(t)-1], true
		}
		return "", false
	}
	if in, ok := inner("LowCardinality"); ok {
		t = in
	}
	if in, ok := inner("Nullable"); ok {
		f, ok := Check(in)
		return func(v string) bool { return v == "" || v == "<nil>" || f(v) }, ok
	}
	if _, ok := inner("Array"); ok {
		return func(v string) bool { return strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") }, true
	}
	for _, e := range []string{"Enum8", "Enum16"} {
		if in, ok := inner(e); ok {
			names := map[string]bool{}
			for _, m := range enumName.FindAllStringSubmatch(in, -1) {
				names[m[1]] = true
			}
			return func(v string) bool { return names[v] }, len(names) > 0
		}
	}
	switch {
	case t == "String", t == "UUID", strings.HasPrefix(t, "FixedString("):
		return func(string) bool { return true }, true
	case t == "DateTime", t == "Date", strings.HasPrefix(t, "DateTime("):
		return GoTime.MatchString, true
	case t == "Bool":
		return func(v string) bool { return v == "true" || v == "false" }, true
	case t == "Float32", t == "Float64":
		return isFloat, true
	case strings.HasPrefix(t, "UInt"):
		return isDigits, true
	case strings.HasPrefix(t, "Int"):
		return func(v string) bool { return isDigits(strings.TrimPrefix(v, "-")) }, true
	}
	return nil, false
}

func isDigits(v string) bool {
	if v == "" {
		return false
	}
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	return true
}

func isFloat(v string) bool {
	if v == "" {
		return false
	}
	if isDigits(v) {
		return true
	}
	_, err := strconv.ParseFloat(v, 64)
	return err == nil
}
