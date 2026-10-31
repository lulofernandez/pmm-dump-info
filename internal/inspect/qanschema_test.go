package inspect

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestQANSchemasEmbedded(t *testing.T) {
	if len(QANSchemas.Schemas) == 0 || QANSchemas.LatestRelease == "" || QANSchemas.Checked == "" {
		t.Fatalf("qan_columns.txt incomplete: %+v", QANSchemas)
	}
	for v, want := range map[string]string{
		"3.9.1": "3.6.0", "3.2.0": "3.1.0", "3.0.0": "3.0.0", "2.41.0": "2.33.0", "2.12.0": "2.11.0", "3.99.0": "3.6.0",
	} {
		if s := QANSchemas.forVersion(v); s == nil || s.Since != want {
			t.Errorf("forVersion(%s) = %v, want since %s", v, s, want)
		}
	}
	if !QANSchemas.newerThanRelease("3.99.0") || QANSchemas.newerThanRelease("3.0.0") {
		t.Error("newerThanRelease")
	}
}

var enumFirst = regexp.MustCompile(`'([^']*)'=`)

// sampleRow is a valid exported row for a schema.
func sampleRow(s *QANSchema, service string) []string {
	var rec []string
	for _, c := range s.Cols {
		v := "x"
		switch {
		case c.Name == "service_name":
			v = service
		case strings.HasPrefix(c.Type, "DateTime"):
			v = "2026-10-30 10:00:00 +0000 UTC"
		case strings.HasPrefix(c.Type, "Enum"):
			v = enumFirst.FindStringSubmatch(c.Type)[1]
		case strings.HasPrefix(c.Type, "Array"):
			v = "[]"
		case strings.HasPrefix(c.Type, "Float"):
			v = "0.25"
		case strings.HasPrefix(c.Type, "UInt"):
			v = "1"
		}
		rec = append(rec, v)
	}
	return rec
}

func TestQANRowsMatch(t *testing.T) {
	s := QANSchemas.forVersion("3.9.1")
	q := newQANScanner()
	q.row("0.tsv", 1, sampleRow(s, "db1"))
	q.row("0.tsv", 2, sampleRow(s, "db2"))
	sum, err := q.summary("3.9.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Services) != 2 || sum.First == 0 || !strings.Contains(sum.Schema, "since PMM 3.6.0") {
		t.Errorf("unexpected %+v", sum)
	}
}

func TestQANRowsShifted(t *testing.T) {
	s := QANSchemas.forVersion("3.9.1")
	rec := sampleRow(s, "db1")
	p := s.index("period_start")
	rec[p], rec[p+1] = rec[p+1], rec[p] // period_start <-> period_length
	q := newQANScanner()
	q.row("0.tsv", 1, rec)
	var qe *QANSchemaError
	if _, err := q.summary("3.9.1"); !errors.As(err, &qe) || !strings.Contains(qe.Detail, "period_start") {
		t.Fatalf("want a QANSchemaError on period_start, got %v", err)
	}
}

func TestQANRowsUnknownCount(t *testing.T) {
	s := QANSchemas.forVersion("3.9.1")
	q := newQANScanner()
	q.row("0.tsv", 1, append(sampleRow(s, "db1"), "0", "0")) // a future PMM with 2 more columns
	var qe *QANSchemaError
	if _, err := q.summary("3.11.0"); !errors.As(err, &qe) {
		t.Fatalf("want a QANSchemaError, got %v", err)
	}
}

func TestQANWrongVersionSchema(t *testing.T) {
	// rows of PMM 2.44 (228 columns) in a dump that says PMM 3.9.1
	q := newQANScanner()
	q.row("0.tsv", 1, sampleRow(QANSchemas.forVersion("2.44.0"), "db1"))
	var qe *QANSchemaError
	if _, err := q.summary("3.9.1"); !errors.As(err, &qe) {
		t.Fatalf("want a QANSchemaError, got %v", err)
	}
}

func TestQANSameCountByEnum(t *testing.T) {
	// 3.1.0 and 3.3.0 have the same columns, 3.3.0 adds a mongolog agent_type
	s := QANSchemas.forVersion("3.3.0")
	rec := sampleRow(s, "db1")
	rec[s.index("agent_type")] = "qan-mongodb-mongolog-agent"
	q := newQANScanner()
	q.row("0.tsv", 1, rec)
	if _, err := q.summary("3.4.0"); err != nil {
		t.Fatal(err)
	}
	q = newQANScanner()
	q.row("0.tsv", 1, rec)
	if _, err := q.summary("3.2.0"); err == nil {
		t.Fatal("a mongolog row cannot come from PMM 3.2.0")
	}
}
