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
