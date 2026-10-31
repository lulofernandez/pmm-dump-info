package inspect

import (
	"fmt"
	"time"
)

// tolerance before a gap between the requested and the real range is reported.
const rangeSlack = 10 * time.Minute

func alerts(r *Result) []Alert {
	var out []Alert
	warn := func(f string, a ...any) { out = append(out, Alert{"warn", fmt.Sprintf(f, a...)}) }
	info := func(f string, a ...any) { out = append(out, Alert{"info", fmt.Sprintf(f, a...)}) }

	// QAN
	switch {
	case r.Args.DumpQAN && len(r.QAN.Files) == 0:
		msg := "QAN was requested (--dump-qan) but the dump has no QAN data (no ch/ files)"
		if r.Log.CHRows != nil {
			msg += fmt.Sprintf("; log.json: ClickHouse returned %d rows for the filter and period", *r.Log.CHRows)
		}
		warn("%s", msg)
	case r.Args.DumpQAN && r.Deep && !r.QAN.Skipped && r.QAN.Rows == 0:
		warn("QAN was requested and there are %d ch/ files, but they hold no rows", len(r.QAN.Files))
	case !r.Args.DumpQAN && len(r.QAN.Files) == 0:
		info("QAN not included: the export was done without --dump-qan")
	}
	if r.QAN.Skipped && len(r.QAN.Files) > 0 {
		info("QAN rows not read (-skip-qan): only the ch/ files and their size are shown")
	}
	if r.Deep && r.QAN.Rows > 0 && QANSchemas.newerThanRelease(r.Meta.PMMServerVersion) {
		warn("Dump from PMM %s, newer than the latest release the QAN schemas know (%s, checked %s). "+
			"The rows match the newest known schema; refresh them with tools/update-qan-schema.sh",
			r.Meta.PMMServerVersion, QANSchemas.LatestRelease, QANSchemas.Checked)
	}
	if len(r.QAN.Files) > 0 && QANSchemas.stale(time.Now()) {
		info("QAN schemas last checked on %s, more than %d days ago: run tools/update-qan-schema.sh --check",
			QANSchemas.Checked, int(staleAfter.Hours()/24))
	}

	// core metrics
	if r.Args.DumpCore && len(r.VM.Chunks) == 0 {
		warn("Core metrics were requested but the dump has no vm/ chunks")
	}
	if r.VM.Format == "native" {
		info("Metrics are in VictoriaMetrics native format (--vm-native-data): services and real time range are not decoded, only the chunk time range")
	}

	// requested vs real range
	if r.Deep && r.VM.Last > 0 {
		if t, err := time.Parse(time.RFC3339, r.Args.Start); err == nil && time.UnixMilli(r.VM.First).Sub(t) > rangeSlack {
			warn("Metrics start at %s, later than the requested --start-ts %s", fmtTime(r.VM.First), r.Args.Start)
		}
		if t, err := time.Parse(time.RFC3339, r.Args.End); err == nil && t.Sub(time.UnixMilli(r.VM.Last)) > rangeSlack {
			warn("Metrics end at %s, earlier than the requested --end-ts %s", fmtTime(r.VM.Last), r.Args.End)
		}
	}

	// --instance filters with nothing behind them
	if r.Deep && r.VM.Format == "json" {
		for _, in := range r.Args.Instances {
			found := false
			for _, s := range r.VM.Services {
				if s.Name == in || s.Node == in {
					found = true
					break
				}
			}
			if !found {
				warn("--instance=%s was requested but there are no metrics for it", in)
			}
		}
	}

	// services with gaps: chunks without samples inside their own range
	if r.Deep {
		for _, s := range r.VM.Services {
			first, last, empty := -1, -1, 0
			for i, n := range s.Coverage {
				if n > 0 {
					if first < 0 {
						first = i
					}
					last = i
				}
			}
			for i := first; i >= 0 && i <= last; i++ {
				if s.Coverage[i] == 0 {
					empty++
				}
			}
			if empty > 0 {
				warn("%s %s has no samples in %d of its %d chunks (gaps in collection)", s.Kind, s.Name, empty, last-first+1)
			}
		}
	}

	// services with metrics but no QAN, when there is QAN
	// (pmm-server-postgresql is PMM's own database, it never has QAN)
	if r.Deep && len(r.QAN.Services) > 0 {
		have := map[string]bool{}
		for _, q := range r.QAN.Services {
			have[q.Name] = true
		}
		for _, s := range r.VM.Services {
			if s.Kind != "node" && s.Kind != "-" && s.Name != "pmm-server-postgresql" && !have[s.Name] {
				info("%s %s has metrics but no QAN", s.Kind, s.Name)
			}
		}
	}

	if n := r.Log.Levels["error"] + r.Log.Levels["fatal"]; n > 0 {
		warn("log.json has %d error lines (see Export log)", n)
	}
	if n := r.Log.Levels["warn"]; n > 0 {
		info("log.json has %d warning lines (see Export log)", n)
	}
	if !r.Log.Present {
		info("No log.json in the dump")
	}
	for _, e := range append(r.VM.Errors, r.QAN.Errors...) {
		warn("Read error: %s", e)
	}
	return out
}

func fmtTime(ms int64) string {
	if ms == 0 {
		return "-"
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02 15:04:05Z")
}
