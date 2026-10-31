package inspect

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// The ch/*.tsv files are "SELECT * FROM metrics" of the PMM ClickHouse,
// written by pmm-dump with encoding/csv (tab separated, no header). The
// column order is the one of pmm.metrics in the exporting PMM, which is only
// known at the end (meta.json is the last tar entry): every row is checked
// against all known schemas with its column count, and summary picks the one
// of the PMM version. A row that matches none stops the scan.

// QANSchemaError means the QAN rows do not match the known pmm.metrics
// layouts: qan_columns.txt is out of date for this dump.
type QANSchemaError struct {
	PMMVersion string
	Columns    int
	Detail     string // first mismatch
}

func (e *QANSchemaError) Error() string {
	ver := e.PMMVersion
	if ver == "" {
		ver = "version unknown"
	}
	set := QANSchemas
	return fmt.Sprintf("unknown QAN column layout: %s\n"+
		"  dump: PMM %s, %d columns per ch/ row\n"+
		"  known pmm.metrics schemas: %s, latest release %s, checked %s\n"+
		"  update them with tools/update-qan-schema.sh and rebuild (./build-mac.sh);\n"+
		"  meanwhile -skip-qan reports everything except QAN",
		e.Detail, ver, e.Columns, set.Source, set.LatestRelease, set.Checked)
}

type qanScanner struct {
	s     QANSummary
	cands []*qanCandidate
	fail  string // why the rows match no schema; stops reading ch/
}

// qanCandidate is one schema the rows may follow, with its own totals.
type qanCandidate struct {
	schema                               *QANSchema
	service, node, kind, queryid, period int
	services                             map[string]*qanAcc
	fail                                 string // first mismatch
}

type qanAcc struct {
	QANService
	queries map[string]bool
}

func newQANScanner() *qanScanner { return &qanScanner{} }

func (q *qanScanner) add(name string, size int64, rd io.Reader, deep bool) {
	q.s.Files = append(q.s.Files, name)
	q.s.Bytes += size
	if !deep || size == 0 || q.fail != "" {
		return
	}
	cr := csv.NewReader(rd)
	cr.Comma = '\t'
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	cr.ReuseRecord = true
	for n := 1; q.fail == ""; n++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			if len(q.s.Errors) < 20 {
				q.s.Errors = append(q.s.Errors, fmt.Sprintf("ch/%s: %v", name, err))
			}
			return
		}
		q.row(name, n, rec)
	}
}

func (q *qanScanner) row(file string, n int, rec []string) {
	q.s.Rows++
	if q.cands == nil {
		q.s.Columns = len(rec)
		for _, s := range QANSchemas.withColumns(len(rec)) {
			q.cands = append(q.cands, &qanCandidate{
				schema: s, service: s.index("service_name"), node: s.index("node_name"),
				kind: s.index("service_type"), queryid: s.index("queryid"), period: s.index("period_start"),
				services: map[string]*qanAcc{},
			})
		}
		if len(q.cands) == 0 {
			var counts []string
			for _, s := range QANSchemas.Schemas {
				counts = append(counts, fmt.Sprintf("%d (PMM %s)", len(s.Cols), s.Since))
			}
			q.fail = fmt.Sprintf("ch/%s has %d columns, no known schema has that many (known: %s)",
				file, len(rec), strings.Join(counts, ", "))
			return
		}
	}
	if len(rec) != q.s.Columns {
		q.fail = fmt.Sprintf("ch/%s row %d has %d columns, the first row had %d", file, n, len(rec), q.s.Columns)
		return
	}
	alive := false
	for _, c := range q.cands {
		if c.fail != "" {
			continue
		}
		if c.fail = c.check(file, n, rec); c.fail == "" {
			c.add(rec)
			alive = true
		}
	}
	if !alive {
		q.fail = q.cands[0].fail // newest schema
	}
}

func (c *qanCandidate) check(file string, n int, rec []string) string {
	for i, col := range c.schema.Cols {
		if !col.check(rec[i]) {
			v := rec[i]
			if len(v) > 60 {
				v = v[:60] + "..."
			}
			return fmt.Sprintf("ch/%s row %d column %d (%s %s) holds %q", file, n, i+1, col.Name, col.Type, v)
		}
	}
	return ""
}

func (c *qanCandidate) add(rec []string) {
	ts, _ := parseGoTime(rec[c.period])
	svc := rec[c.service]
	if svc == "" {
		svc = "(unknown service)"
	}
	a := c.services[svc]
	if a == nil {
		a = &qanAcc{queries: map[string]bool{}}
		a.Name, a.Kind = svc, rec[c.kind]
		if c.node >= 0 {
			a.Node = rec[c.node]
		}
		a.Buckets = map[int64]int64{}
		c.services[svc] = a
	}
	a.Rows++
	if id := rec[c.queryid]; id != "" {
		a.queries[id] = true
	}
	if ts > 0 {
		a.Buckets[ts]++
		if a.First == 0 || ts < a.First {
			a.First = ts
		}
		if ts > a.Last {
			a.Last = ts
		}
	}
}

// summary picks the schema of the exporting PMM version; it is an error when
// the rows do not follow it.
func (q *qanScanner) summary(pmm string) (QANSummary, error) {
	s := q.s
	sort.Strings(s.Files)
	if q.fail != "" {
		return s, &QANSchemaError{pmm, s.Columns, q.fail}
	}
	if len(q.cands) == 0 {
		return s, nil
	}
	var pick *qanCandidate
	note := ""
	if want := QANSchemas.forVersion(pmm); want != nil {
		for _, c := range q.cands {
			if c.schema == want {
				pick = c
			}
		}
		if pick == nil {
			return s, &QANSchemaError{pmm, s.Columns, fmt.Sprintf("PMM %s has pmm.metrics %s, the rows have %d columns", pmm, want, s.Columns)}
		}
		if pick.fail != "" {
			return s, &QANSchemaError{pmm, s.Columns, fmt.Sprintf("PMM %s has pmm.metrics %s, but %s", pmm, want, pick.fail)}
		}
	} else {
		for _, c := range q.cands {
			if c.fail == "" {
				pick = c
				break
			}
		}
		note = fmt.Sprintf(" (PMM version %q not recognized, picked by column count and types)", pmm)
	}
	s.Schema = fmt.Sprintf("pmm.metrics %s, all rows match its column types%s", pick.schema, note)
	for _, a := range pick.services {
		a.Queries = len(a.queries)
		if a.First != 0 && (s.First == 0 || a.First < s.First) {
			s.First = a.First
		}
		if a.Last > s.Last {
			s.Last = a.Last
		}
		s.Services = append(s.Services, a.QANService)
	}
	sort.Slice(s.Services, func(i, j int) bool { return s.Services[i].Name < s.Services[j].Name })
	return s, nil
}
