// Package report writes the inspection result as text or as a self-contained
// HTML page.
package report

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"pmm-dump-info/internal/inspect"
)

// WriteText prints the report for a terminal; color adds ANSI styles to the
// titles, column headers and alerts.
func WriteText(w io.Writer, r *inspect.Result, color bool) {
	p := func(f string, a ...any) { fmt.Fprintf(w, f, a...) }
	c := palette{color}
	m := r.Meta

	p("\n%s  %s (%s)\n\n", c.paint(styleTitle, "PMM-DUMP-INFO"), r.Name, bytesStr(r.Size))

	var kv [][]string
	row := func(k, v string) { kv = append(kv, []string{k, v}) }
	p("%s\n", c.paint(styleTitle, "Dump"))
	row("pmm-dump", r.PMMDump)
	row("PMM Server", m.PMMServerVersion)
	row("Exported from", r.Args.Source)
	tz := "-"
	if m.PMMTimezone != nil {
		tz = *m.PMMTimezone
	}
	row("PMM timezone", tz)
	row("Encrypted", yesNo(r.Encrypted))
	row("Requested range", r.Args.Range)
	row("Core metrics", yesNo(r.Args.DumpCore)+" (format "+r.VM.Format+")")
	row("QAN", yesNo(r.Args.DumpQAN))
	if len(r.Args.Instances) > 0 {
		row("--instance", strings.Join(r.Args.Instances, ", "))
	}
	if len(r.Args.Dashboards) > 0 {
		row("--dashboard", strings.Join(r.Args.Dashboards, ", "))
	}
	if r.Args.TSSelector != "" {
		row("--ts-selector", r.Args.TSSelector)
	}
	if r.Args.Where != "" {
		row("--where", r.Args.Where)
	}
	if len(m.PMMServerServices) > 0 {
		var names []string
		for _, s := range m.PMMServerServices {
			names = append(names, s.Name+" ("+s.NodeName+")")
		}
		row("Services in meta", strings.Join(names, ", "))
	}
	table(w, c, false, true, kv)

	if len(r.Alerts) > 0 {
		p("\n%s\n", c.paint(styleTitle, "Alerts"))
		for _, a := range r.Alerts {
			if a.Level == "warn" {
				p("  %s %s\n", c.paint(styleWarn, "!"), a.Text)
			} else {
				p("  %s %s\n", c.paint(styleInfo, "i"), a.Text)
			}
		}
	}

	vm := r.VM
	p("\n%s %d chunks, %s", c.paint(styleTitle, "Metrics (vm/):"), len(vm.Chunks), bytesStr(vm.Bytes))
	if len(vm.Chunks) > 0 {
		p(", chunks cover %s -> %s", fmtTime(vm.Chunks[0].Start), fmtTime(vm.Chunks[len(vm.Chunks)-1].End))
	}
	p("\n")
	if r.Deep && vm.Format == "json" && len(vm.Chunks) > 0 {
		p("  real data %s -> %s, %s samples, %s distinct series\n\n",
			fmtTime(vm.First), fmtTime(vm.Last), countStr(vm.Samples), countStr(int64(vm.Series)))
		rows := [][]string{{"SERVICE", "TYPE", "NODE", "FIRST", "LAST", "SERIES", "SAMPLES", "METRIC FAMILIES"}}
		for _, s := range vm.Services {
			rows = append(rows, []string{s.Name, s.Kind, s.Node,
				fmtTime(s.First), fmtTime(s.Last), countStr(int64(s.Series)), countStr(s.Samples), families(s.Families, 5)})
		}
		table(w, c, true, false, rows)
	} else if !r.Deep {
		p("  (quick mode: chunk contents not read; use -deep for services and real range)\n")
	}

	q := r.QAN
	p("\n%s %d files, %s", c.paint(styleTitle, "QAN (ch/):"), len(q.Files), bytesStr(q.Bytes))
	if r.Deep && len(q.Files) > 0 {
		p(", %s rows, %s -> %s\n  %s\n", countStr(q.Rows), fmtTime(q.First), fmtTime(q.Last), q.Schema)
		if len(q.Services) > 0 {
			p("\n")
			rows := [][]string{{"SERVICE", "TYPE", "NODE", "FIRST", "LAST", "ROWS", "QUERIES"}}
			for _, s := range q.Services {
				rows = append(rows, []string{s.Name, orDash(s.Kind), orDash(s.Node),
					fmtTime(s.First), fmtTime(s.Last), countStr(s.Rows), fmt.Sprint(s.Queries)})
			}
			table(w, c, true, false, rows)
		}
	} else {
		p("\n")
	}

	p("\n%s\n  %s\n", c.paint(styleTitle, "Export command:"), r.Args.Masked)
	if r.Log.Present {
		p("\n%s %d lines, %s -> %s%s, levels %s", c.paint(styleTitle, "Export log (log.json):"), r.Log.Lines, orDash(r.Log.First), orDash(r.Log.Last), logDuration(r.Log.First, r.Log.Last), levels(r.Log.Levels))
		if len(r.Log.Notable) == 0 {
			p(", no warnings or errors\n")
		} else {
			p("\n")
		}
		for i, l := range r.Log.Notable {
			if i == 15 {
				p("  ... %d more in the HTML report\n", len(r.Log.Notable)-15)
				break
			}
			p("  %s\n", clip(l, 240))
		}
	}
	if len(r.Other) > 0 {
		p("\n%s %s\n", c.paint(styleTitle, "Other files:"), strings.Join(r.Other, ", "))
	}
}

func families(f map[string]int, top int) string {
	type kv struct {
		k string
		v int
	}
	var l []kv
	for k, v := range f {
		l = append(l, kv{k, v})
	}
	sort.Slice(l, func(i, j int) bool { return l[i].v > l[j].v || (l[i].v == l[j].v && l[i].k < l[j].k) })
	var out []string
	for i, x := range l {
		if i == top {
			out = append(out, fmt.Sprintf("+%d", len(l)-top))
			break
		}
		out = append(out, fmt.Sprintf("%s %d", x.k, x.v))
	}
	return strings.Join(out, ", ")
}

func levels(m map[string]int) string {
	var out []string
	for _, k := range []string{"fatal", "error", "warn", "info", "debug"} {
		if m[k] > 0 {
			out = append(out, fmt.Sprintf("%s %d", k, m[k]))
		}
	}
	return strings.Join(out, ", ")
}

func fmtTime(ms int64) string {
	if ms == 0 {
		return "-"
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02 15:04:05Z")
}

func bytesStr(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}

func countStr(n int64) string {
	switch {
	case n >= 1e9:
		return fmt.Sprintf("%.1fG", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1e4:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// logDuration is " (3s)" between two RFC 3339 times, or "" when they do not parse.
func logDuration(first, last string) string {
	a, err1 := time.Parse(time.RFC3339, first)
	b, err2 := time.Parse(time.RFC3339, last)
	if err1 != nil || err2 != nil {
		return ""
	}
	return " (" + b.Sub(a).Round(time.Second).String() + ")"
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
