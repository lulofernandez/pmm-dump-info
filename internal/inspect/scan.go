package inspect

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"pmm-dump-info/internal/dump"
)

// Scan reads the whole dump once. With deep=false only meta.json, log.json
// and the tar index are read; chunk contents are skipped. skipQAN skips only
// the ch/ contents. A *QANSchemaError means the QAN rows follow no known
// pmm.metrics layout.
func Scan(file, pass string, deep, skipQAN bool, workers int) (*Result, error) {
	d, err := dump.Open(file, pass)
	if err != nil {
		return nil, err
	}
	defer d.Close()

	r := &Result{File: file, Encrypted: d.Encrypted, Deep: deep}
	if st, err := os.Stat(file); err == nil {
		r.Size = st.Size()
	}
	vm := newVMScanner(workers)
	qan := newQANScanner()
	for {
		h, err := d.Tar.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading the tar: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		dir, name := path.Split(h.Name)
		switch {
		case strings.HasPrefix(name, "._"): // macOS AppleDouble, from a dump repacked on a Mac
			r.Other = append(r.Other, h.Name)
		case dir == "" && name == "meta.json":
			r.Meta = &Meta{}
			if err := json.NewDecoder(d.Tar).Decode(r.Meta); err != nil {
				return nil, fmt.Errorf("meta.json: %w", err)
			}
		case dir == "" && name == "log.json":
			r.Log = readLog(d.Tar)
		case dir == "vm/":
			vm.add(name, h.Size, d.Tar, deep)
		case dir == "ch/":
			qan.add(name, h.Size, d.Tar, deep && !skipQAN)
		default:
			r.Other = append(r.Other, h.Name)
		}
	}
	if r.Meta == nil {
		return nil, fmt.Errorf("%s: no meta.json, this does not look like a pmm-dump file", file)
	}
	r.Args = ParseArgs(r.Meta.Arguments)
	r.VM = vm.summary(r.Meta.VMDataFormat)
	if r.QAN, err = qan.summary(r.Meta.PMMServerVersion); err != nil {
		return nil, err
	}
	r.QAN.Skipped = skipQAN
	r.Alerts = alerts(r)
	return r, nil
}

// ---- log.json ----

func readLog(rd io.Reader) LogSummary {
	ls := LogSummary{Present: true, Levels: map[string]int{}}
	seen := map[string]bool{}
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Bytes()
		var e map[string]any
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		ls.Lines++
		level, _ := e["level"].(string)
		msg, _ := e["message"].(string)
		t, _ := e["time"].(string)
		ls.Levels[level]++
		if ls.First == "" {
			ls.First = t
		}
		ls.Last = t
		if msg == "Split Click House rows into chunks" {
			if v, ok := e["rows"].(float64); ok {
				n := int64(v)
				ls.CHRows = &n
			}
		}
		notable := level == "warn" || level == "error" || level == "fatal" || level == "panic"
		if notable && !seen[level+msg] && len(ls.Notable) < 50 {
			seen[level+msg] = true
			ls.Notable = append(ls.Notable, fmt.Sprintf("%s %s %s", t, strings.ToUpper(level), logText(e)))
		}
	}
	return ls
}

// logText is the message plus the extra fields, without secrets.
func logText(e map[string]any) string {
	var keys []string
	for k := range e {
		if k != "level" && k != "time" && k != "message" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	s := fmt.Sprint(e["message"])
	for _, k := range keys {
		v := fmt.Sprint(e[k])
		if len(v) > 200 {
			v = v[:200] + "..."
		}
		s += fmt.Sprintf(" %s=%s", k, v)
	}
	return maskURL(s)
}

// ---- vm/ chunks ----

type svcKey struct {
	service, node string
	internal      bool // PMM Server's own components (instance="pmm-server", no node/service)
}

type svcAcc struct {
	Service
	series   map[uint64]bool
	coverage map[int]int64 // chunk index -> samples
}

type vmScanner struct {
	chunks   []*Chunk
	services map[svcKey]*svcAcc
	series   map[uint64]bool
	native   bool
	errors   []string

	mu  sync.Mutex
	wg  sync.WaitGroup
	sem chan struct{}
}

func newVMScanner(workers int) *vmScanner {
	return &vmScanner{services: map[svcKey]*svcAcc{}, series: map[uint64]bool{}, sem: make(chan struct{}, max(workers, 1))}
}

// add records a chunk; with deep it reads the (compressed) content and parses
// it in a worker, chunks are independent.
func (v *vmScanner) add(name string, size int64, rd io.Reader, deep bool) {
	c := &Chunk{Name: name, Bytes: size}
	if s, e, ok := strings.Cut(strings.TrimSuffix(name, ".bin"), "-"); ok {
		c.Start, _ = strconv.ParseInt(s, 10, 64)
		c.End, _ = strconv.ParseInt(e, 10, 64)
		c.Start *= 1000
		c.End *= 1000
	}
	idx := len(v.chunks)
	v.chunks = append(v.chunks, c)
	if !deep {
		return
	}
	data, err := io.ReadAll(rd)
	if err != nil {
		v.fail(name, err)
		return
	}
	v.sem <- struct{}{}
	v.wg.Add(1)
	go func() {
		defer func() { <-v.sem; v.wg.Done() }()
		recs, native, err := parseChunk(c, data)
		v.mu.Lock()
		defer v.mu.Unlock()
		if err != nil {
			v.failLocked(name, err)
		}
		if native {
			v.native = true
		}
		for _, r := range recs {
			v.addSeries(idx, r)
		}
	}()
}

func (v *vmScanner) fail(name string, err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.failLocked(name, err)
}

func (v *vmScanner) failLocked(name string, err error) {
	if len(v.errors) < 20 {
		v.errors = append(v.errors, fmt.Sprintf("vm/%s: %v", name, err))
	}
}

// line is one series of the VictoriaMetrics /api/v1/export JSON format.
// Values are not needed; timestamps are scanned by hand (count, min, max).
type line struct {
	Metric     map[string]string `json:"metric"`
	Timestamps json.RawMessage   `json:"timestamps"`
}

// rec is one series of a chunk, reduced to what the summary needs.
type rec struct {
	key         svcKey
	stype, node string
	fam         string
	hash        uint64
	n, lo, hi   int64
}

func parseChunk(c *Chunk, data []byte) ([]rec, bool, error) {
	var r io.Reader = bytes.NewReader(data)
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return nil, false, err
		}
		defer gz.Close()
		r = gz
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 4<<20), 256<<20)
	var recs []rec
	first := true
	for sc.Scan() {
		b := sc.Bytes()
		if len(b) == 0 {
			continue
		}
		if first && b[0] != '{' {
			return nil, true, nil // --vm-native-data: binary, not decoded
		}
		first = false
		labels, ts, ok := fastLine(b)
		if !ok {
			var l line
			if err := json.Unmarshal(b, &l); err != nil {
				return recs, false, err
			}
			labels, ts = l.Metric, l.Timestamps
		}
		n, lo, hi := scanTimestamps(ts)
		c.Lines++
		c.Samples += n
		k := svcKey{service: labels["service_name"], node: labels["node_name"]}
		if k.service != "" {
			k.node = "" // a service is one row even if its node label varies
		}
		fam := family(labels["__name__"])
		if k.service == "" && k.node == "" && labels["instance"] == "pmm-server" {
			// PMM scraping its own components (grafana, clickhouse, victoriametrics...): one row, by job
			k.internal = true
			fam = labels["job"]
		}
		recs = append(recs, rec{k, labels["service_type"], labels["node_name"], fam, hashLabels(labels), n, lo, hi})
	}
	return recs, false, sc.Err()
}

var (
	metricPrefix = []byte(`{"metric":{`)
	tsKey        = []byte(`"timestamps":[`)
)

// fastLine parses the labels and finds the timestamps of a line shaped like
// {"metric":{"k":"v",...},"values":[...],"timestamps":[...]} without
// decoding the values, which is most of the line. Anything unexpected
// returns ok=false and the caller falls back to encoding/json.
func fastLine(b []byte) (map[string]string, []byte, bool) {
	if !bytes.HasPrefix(b, metricPrefix) {
		return nil, nil, false
	}
	m := make(map[string]string, 16)
	i := len(metricPrefix)
	for i < len(b) && b[i] != '}' {
		k, n, ok := jsonString(b[i:])
		if !ok || i+n >= len(b) || b[i+n] != ':' {
			return nil, nil, false
		}
		i += n + 1
		v, n, ok := jsonString(b[i:])
		if !ok {
			return nil, nil, false
		}
		m[k] = v
		i += n
		if i < len(b) && b[i] == ',' {
			i++
		}
	}
	t := bytes.LastIndex(b, tsKey)
	if i >= len(b) || t < 0 {
		return nil, nil, false
	}
	return m, b[t+len(tsKey)-1:], true
}

// jsonString reads a JSON string at the start of b and returns it with the
// number of bytes consumed.
func jsonString(b []byte) (string, int, bool) {
	if len(b) == 0 || b[0] != '"' {
		return "", 0, false
	}
	esc := false
	for j := 1; j < len(b); j++ {
		switch b[j] {
		case '\\':
			esc = true
			j++
		case '"':
			if !esc {
				return string(b[1:j]), j + 1, true
			}
			var s string
			if err := json.Unmarshal(b[:j+1], &s); err != nil {
				return "", 0, false
			}
			return s, j + 1, true
		}
	}
	return "", 0, false
}

func (v *vmScanner) addSeries(chunk int, r rec) {
	a := v.services[r.key]
	if a == nil {
		a = &svcAcc{series: map[uint64]bool{}, coverage: map[int]int64{}}
		a.Families = map[string]int{}
		a.Name, a.Node = r.key.service, r.node
		if r.key.internal {
			a.Name, a.Kind = "PMM Server internals", "pmm-server"
		} else if r.key.service == "" {
			a.Name, a.Kind = r.node, "node"
			if a.Name == "" {
				a.Name = "(no service/node label)"
			}
		}
		v.services[r.key] = a
	}
	if a.Kind == "" && r.stype != "" {
		a.Kind = strings.TrimSuffix(r.stype, "_service")
	}
	if a.Node == "" {
		a.Node = r.node
	}
	v.series[r.hash] = true
	if !a.series[r.hash] {
		a.series[r.hash] = true
		a.Families[r.fam]++
	}
	a.Samples += r.n
	a.coverage[chunk] += r.n
	if r.n > 0 {
		if a.First == 0 || r.lo < a.First {
			a.First = r.lo
		}
		if r.hi > a.Last {
			a.Last = r.hi
		}
	}
}

func (v *vmScanner) summary(format string) VMSummary {
	v.wg.Wait()
	s := VMSummary{Format: format, Errors: v.errors}
	if s.Format == "" {
		s.Format = "json"
	}
	if v.native {
		s.Format = "native"
	}
	order := make([]int, len(v.chunks)) // chunk indexes sorted by start
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return v.chunks[order[i]].Start < v.chunks[order[j]].Start })
	for _, i := range order {
		c := v.chunks[i]
		s.Chunks = append(s.Chunks, *c)
		s.Bytes += c.Bytes
		s.Samples += c.Samples
	}
	s.Series = len(v.series)
	for _, a := range v.services {
		a.Series = len(a.series)
		a.Coverage = make([]int64, len(order))
		for pos, i := range order {
			a.Coverage[pos] = a.coverage[i]
		}
		if a.Kind == "" {
			a.Kind = "-"
		}
		if a.First != 0 && (s.First == 0 || a.First < s.First) {
			s.First = a.First
		}
		if a.Last > s.Last {
			s.Last = a.Last
		}
		s.Services = append(s.Services, a.Service)
	}
	// nodes first, then services, then PMM Server internals and the series without service/node label
	rank := func(a Service) int {
		switch {
		case a.Kind == "node" && a.Node == "":
			return 3
		case a.Kind == "pmm-server":
			return 2
		case a.Kind == "node":
			return 0
		}
		return 1
	}
	sort.Slice(s.Services, func(i, j int) bool {
		x, y := s.Services[i], s.Services[j]
		if rx, ry := rank(x), rank(y); rx != ry {
			return rx < ry
		}
		return x.Name < y.Name
	})
	return s
}

// scanTimestamps returns count, min and max of a JSON array of integers
// without allocating a slice.
func scanTimestamps(raw []byte) (n, lo, hi int64) {
	var cur int64
	in := false
	for _, ch := range raw {
		if ch >= '0' && ch <= '9' {
			cur = cur*10 + int64(ch-'0')
			in = true
			continue
		}
		if in {
			if n == 0 || cur < lo {
				lo = cur
			}
			if cur > hi {
				hi = cur
			}
			n++
			cur, in = 0, false
		}
	}
	return n, lo, hi
}

// family is the metric name up to the first "_": mysql, node, mongodb, pg...
func family(name string) string {
	if i := strings.IndexByte(name, '_'); i > 0 {
		return name[:i]
	}
	if name == "" {
		return "(none)"
	}
	return name
}

// hashLabels is FNV-1a over the sorted label pairs.
func hashLabels(m map[string]string) uint64 {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b bytes.Buffer
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte(0)
		b.WriteString(m[k])
		b.WriteByte(0)
	}
	h := uint64(14695981039346656037)
	for _, c := range b.Bytes() {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}
