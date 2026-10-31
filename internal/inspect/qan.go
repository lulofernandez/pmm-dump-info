package inspect

import (
	_ "embed"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

// The ch/*.tsv files are "SELECT * FROM metrics" of the PMM ClickHouse,
// written by pmm-dump with encoding/csv (tab separated, no header). The
// column order is the one of pmm.metrics in the exporting PMM.
//
//go:embed qan_columns.txt
var qanColumnsFile string

// qanSchemas maps a column count to the column names, from qan_columns.txt.
var qanSchemas = parseSchemas(qanColumnsFile)

func parseSchemas(s string) map[int][]string {
	out := map[int][]string{}
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			out[len(cur)] = cur
		}
		cur = nil
	}
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "#"):
		case l == "":
			flush()
		default:
			cur = append(cur, strings.Fields(l)[0])
		}
	}
	flush()
	return out
}

// pmm-dump writes DateTime values with fmt %v of a time.Time.
var goTime = regexp.MustCompile(`^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d [+-]\d{4} [A-Z]+$`)

func parseGoTime(s string) (int64, bool) {
	t, err := time.Parse("2006-01-02 15:04:05 -0700 MST", s)
	if err != nil {
		return 0, false
	}
	return t.UnixMilli(), true
}

type qanCols struct{ service, node, kind, queryid, period int }

type qanScanner struct {
	s        QANSummary
	cols     *qanCols
	services map[string]*qanAcc
}

type qanAcc struct {
	QANService
	queries map[string]bool
}

func newQANScanner() *qanScanner { return &qanScanner{services: map[string]*qanAcc{}} }

func (q *qanScanner) add(name string, size int64, rd io.Reader, deep bool) {
	q.s.Files = append(q.s.Files, name)
	q.s.Bytes += size
	if !deep || size == 0 {
		return
	}
	cr := csv.NewReader(rd)
	cr.Comma = '\t'
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	cr.ReuseRecord = true
	for {
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
		q.row(rec)
	}
}

func (q *qanScanner) row(rec []string) {
	q.s.Rows++
	if q.s.Columns == 0 {
		q.s.Columns = len(rec)
	}
	if q.cols == nil {
		q.cols = q.identify(rec)
	}
	c := q.cols
	get := func(i int) string {
		if i < 0 || i >= len(rec) {
			return ""
		}
		return rec[i]
	}
	ts, _ := parseGoTime(get(c.period))
	svc := get(c.service)
	if svc == "" {
		svc = "(unknown service)"
	}
	a := q.services[svc]
	if a == nil {
		a = &qanAcc{queries: map[string]bool{}}
		a.Name, a.Node, a.Kind = svc, get(c.node), get(c.kind)
		a.Buckets = map[int64]int64{}
		q.services[svc] = a
	}
	a.Rows++
	if id := get(c.queryid); id != "" {
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

// identify finds the columns: by the known schema with the same column count
// if its period_start looks like a date, otherwise only period_start by shape.
func (q *qanScanner) identify(rec []string) *qanCols {
	c := &qanCols{-1, -1, -1, -1, -1}
	if names, ok := qanSchemas[len(rec)]; ok {
		idx := map[string]int{}
		for i, n := range names {
			idx[n] = i
		}
		if p, ok := idx["period_start"]; ok && goTime.MatchString(rec[p]) {
			c.service, c.node, c.kind, c.queryid, c.period = idx["service_name"], idx["node_name"], idx["service_type"], idx["queryid"], p
			q.s.Schema = fmt.Sprintf("known pmm.metrics schema (%d columns)", len(rec))
			return c
		}
	}
	for i, v := range rec {
		if goTime.MatchString(v) {
			c.period = i
			break
		}
	}
	q.s.Schema = fmt.Sprintf("unknown pmm.metrics schema (%d columns): only rows and time range, no per-service split", len(rec))
	return c
}

func (q *qanScanner) summary() QANSummary {
	s := q.s
	for _, a := range q.services {
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
	sort.Strings(s.Files)
	return s
}
