package inspect

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestParseArgsUI(t *testing.T) {
	a := ParseArgs("export --pmm-url=http://127.0.0.1:8080 --click-house-url=tcp://default:clickhouse@127.0.0.1:9000/pmm " +
		"--dump-path=/srv/dump/x.tar.gz --pmm-cookie=d727f2 --instance=a,b,a --instance=b,a " +
		"--start-ts=2026-10-29T17:19:00Z --end-ts=2026-10-30T05:19:00Z --dump-qan=true --encryption=false")
	if strings.Contains(a.Masked, "clickhouse@") || strings.Contains(a.Masked, "d727f2") {
		t.Errorf("secret not masked: %s", a.Masked)
	}
	if !reflect.DeepEqual(a.Instances, []string{"a", "b"}) {
		t.Errorf("instances = %v", a.Instances)
	}
	if strings.Count(a.Masked, "--instance=") != 1 || !strings.Contains(a.Masked, "--instance=a,b ") {
		t.Errorf("instance not deduplicated: %s", a.Masked)
	}
	if !a.DumpQAN || !a.DumpCore || a.Source != "PMM UI (Help > PMM Dump)" || a.Start != "2026-10-29T17:19:00Z" {
		t.Errorf("unexpected %+v", a)
	}
}

func TestParseArgsCLI(t *testing.T) {
	a := ParseArgs(`export --pmm-url=https://admin:secret@1.2.3.4 --dump-qan --no-dump-core --where="service_name='db1' AND x = 1"`)
	if strings.Contains(a.Masked, "secret") {
		t.Errorf("password not masked: %s", a.Masked)
	}
	if !a.DumpQAN || a.DumpCore || a.Source != "pmm-dump CLI" {
		t.Errorf("unexpected %+v", a)
	}
	if a.Where != `service_name='db1' AND x = 1` {
		t.Errorf("where = %q", a.Where)
	}
}

func TestFastLine(t *testing.T) {
	lines := []string{
		`{"metric":{"__name__":"mysql_up","service_name":"db1","node_name":"n1"},"values":[1,1,1],"timestamps":[1000,2000,3000]}`,
		`{"metric":{"__name__":"x","q":"a \"quoted\" \\ value","u":"é"},"values":[0.5],"timestamps":[5]}`,
	}
	for _, l := range lines {
		var want line
		if err := json.Unmarshal([]byte(l), &want); err != nil {
			t.Fatal(err)
		}
		m, ts, ok := fastLine([]byte(l))
		if !ok {
			t.Fatalf("fastLine failed on %s", l)
		}
		if !reflect.DeepEqual(m, want.Metric) {
			t.Errorf("labels %v, want %v", m, want.Metric)
		}
		if string(ts[:len(want.Timestamps)]) != string(want.Timestamps) {
			t.Errorf("timestamps %s, want %s", ts, want.Timestamps)
		}
	}
	if _, _, ok := fastLine([]byte(`{"timestamps":[1],"metric":{}}`)); ok {
		t.Error("unexpected order should fall back to encoding/json")
	}
}

func TestScanTimestamps(t *testing.T) {
	n, lo, hi := scanTimestamps([]byte(`[3000,1000,2000]}`))
	if n != 3 || lo != 1000 || hi != 3000 {
		t.Errorf("got %d %d %d", n, lo, hi)
	}
	if n, _, _ := scanTimestamps([]byte(`[]`)); n != 0 {
		t.Errorf("empty array: %d", n)
	}
}

func TestInstanceNodeWithoutNodeMetrics(t *testing.T) {
	r := &Result{Deep: true}
	r.VM.Format = "json"
	r.Args.Instances = []string{"db1-postgresql", "db1", "db2"}
	r.VM.Services = []Service{{Name: "db1-postgresql", Kind: "postgresql", Node: "db1"}}
	var got []string
	for _, a := range alerts(r) {
		if strings.Contains(a.Text, "--instance") {
			got = append(got, a.Text)
		}
	}
	want := []string{
		"--instance=db1 is a node, but the dump has no node metrics for it (no OS metrics); the pmm-dump of PMM 3.0.0 to 3.7.x and 2.x before 2.43.0 matches --instance only against service_name",
		"--instance=db2 was requested but there are no metrics for it",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("alerts = %q", got)
	}
	nodeAlert := func() string {
		for _, a := range alerts(r) {
			if strings.Contains(a.Text, "db1 is a node") {
				return a.Text
			}
		}
		return ""
	}
	r.Meta = &Meta{}
	r.Args.Source = "PMM UI (Help > PMM Dump)"
	for _, c := range []struct{ pmm, want string }{
		{"3.7.1", "; the pmm-dump of PMM 3.7.1 matches --instance only against service_name (PMM 3.0.0 to 3.7.x and 2.x before 2.43.0), node metrics come only with a node_name selector (--ts-selector)"},
		{"2.42.0", "; the pmm-dump of PMM 2.42.0 matches"},
		{"3.8.0", ""},
		{"2.43.0", ""},
		{"", ""},
	} {
		r.Meta.PMMServerVersion = c.pmm
		got := strings.TrimPrefix(nodeAlert(), "--instance=db1 is a node, but the dump has no node metrics for it (no OS metrics)")
		if (c.want == "" && got != "") || !strings.HasPrefix(got, c.want) {
			t.Errorf("UI %s: hint = %q", c.pmm, got)
		}
	}
	r.Args.Source = "pmm-dump CLI"
	r.Meta.PMMServerVersion = "3.9.1"
	if !strings.HasSuffix(nodeAlert(), "; the pmm-dump of PMM 3.0.0 to 3.7.x and 2.x before 2.43.0 matches --instance only against service_name") {
		t.Errorf("CLI hint = %q", nodeAlert())
	}
	r.VM.Services = append(r.VM.Services, Service{Name: "db1", Kind: "node", Node: "db1"})
	for _, a := range alerts(r) {
		if strings.Contains(a.Text, "db1 is a node") {
			t.Errorf("unexpected alert with node metrics present: %s", a.Text)
		}
	}
}

func TestParseArgsWhereAndRange(t *testing.T) {
	a := ParseArgs(`export --start-ts=2026-08-06T14:34:32Z --instance=lab3 --where=service_name='lab3-db'`)
	if a.Where != `service_name='lab3-db'` {
		t.Errorf("where = %q", a.Where)
	}
	for _, c := range []struct{ args, want string }{
		{`export --pmm-url=http://x`, "not set: the 4 hours before the export (pmm-dump default)"},
		{`export --start-ts=S --end-ts=E`, "S -> E"},
		{`export --end-ts=E`, "end - 4h (default) -> E"},
		{`export --start-ts=S`, "S -> export time (default)"},
	} {
		if got := ParseArgs(c.args).Range; got != c.want {
			t.Errorf("%s: range = %q, want %q", c.args, got, c.want)
		}
	}
}

func TestPMMDumpVersion(t *testing.T) {
	var m Meta
	if v := pmmDumpVersion(&m); v != "-" {
		t.Errorf("empty version = %q", v)
	}
	m.Version.GitCommit = "0d49b27"
	if v := pmmDumpVersion(&m); v != "0d49b27" {
		t.Errorf("commit only = %q", v)
	}
	m.Version.GitBranch = "main"
	if v := pmmDumpVersion(&m); v != "main@0d49b27" {
		t.Errorf("branch and commit = %q", v)
	}
}

func alertTexts(r *Result, sub string) []string {
	var out []string
	for _, a := range alerts(r) {
		if strings.Contains(a.Text, sub) {
			out = append(out, a.Text)
		}
	}
	return out
}

func TestNoSamplesAndExactAlternation(t *testing.T) {
	r := &Result{Deep: true}
	r.VM.Format = "json"
	r.VM.Chunks = make([]Chunk, 3)
	r.Args.TSSelector = `{job=~"postgres_exporter_.*|node_exporter_.*", node_name="n01|n02", __name__!~"a|b"}`
	got := append(alertTexts(r, "no samples"), alertTexts(r, "--ts-selector")...)
	want := []string{
		"The dump has 3 vm/ chunks but no samples: the export filters matched no series",
		`--ts-selector has node_name="n01|n02": with = the | is literal and matches no series, node_name=~ was meant`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("alerts = %q", got)
	}
	r.VM.Samples = 10
	r.Args.TSSelector = `{node_name=~"n01|n02"}`
	if got := append(alertTexts(r, "no samples"), alertTexts(r, "--ts-selector")...); len(got) > 0 {
		t.Errorf("unexpected alerts %q", got)
	}
}

func TestOnlyUpScrape(t *testing.T) {
	r := &Result{Deep: true}
	r.VM.Services = []Service{
		{Name: "cfg-0", Kind: "mongodb", Series: 12, Families: map[string]int{"scrape": 10, "up": 2}},
		{Name: "rs0-0", Kind: "mongodb", Series: 20, Families: map[string]int{"mongodb": 10, "scrape": 8, "up": 2}},
	}
	want := []string{"mongodb cfg-0 has only up/scrape series (12): its exporter returned no metrics in the range (check the up series)"}
	if got := alertTexts(r, "only up/scrape"); !reflect.DeepEqual(got, want) {
		t.Errorf("alerts = %q", got)
	}
}

func TestMetricsButNoQANGrouped(t *testing.T) {
	r := &Result{Deep: true}
	r.QAN.Services = []QANService{{Name: "pmm-server-postgresql"}}
	r.VM.Services = []Service{
		{Name: "proxy-0", Kind: "proxysql"}, {Name: "n1", Kind: "node"},
		{Name: "db1", Kind: "mysql"}, {Name: "rs0", Kind: "mongodb"},
		{Name: "pmm-server-postgresql", Kind: "postgresql"},
	}
	want := []string{"2 services have metrics but no QAN: mysql db1, mongodb rs0"}
	if got := alertTexts(r, "no QAN"); !reflect.DeepEqual(got, want) {
		t.Errorf("alerts = %q", got)
	}
	r.VM.Services = r.VM.Services[:3]
	want = []string{"mysql db1 has metrics but no QAN"}
	if got := alertTexts(r, "no QAN"); !reflect.DeepEqual(got, want) {
		t.Errorf("alerts = %q", got)
	}
}
