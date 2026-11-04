// pmm-dump-info reads a pmm-dump file without importing it into PMM and
// reports versions, export arguments, services with metrics and QAN, and the
// time range covered.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"pmm-dump-info/internal/dump"
	"pmm-dump-info/internal/inspect"
	"pmm-dump-info/internal/report"
)

func main() {
	open := flag.Bool("open", false, "write the HTML report (<dump>.html next to the dump) and open it in the browser instead of text")
	quick := flag.Bool("quick", false, "only meta.json, log.json and the file list; do not read chunk contents")
	skipQAN := flag.Bool("skip-qan", false, "do not read the QAN rows (ch/), e.g. when their column layout is unknown")
	pass := flag.String("pass", "", "password of an encrypted dump (also env PMM_DUMP_PASS)")
	workers := flag.Int("j", 4, "chunks parsed in parallel (keep it low on shared hosts)")
	color := flag.String("color", "auto", "colors in the text report: auto (terminal and no NO_COLOR), always, never")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: pmm-dump-info [flags] <pmm-dump.tar.gz | .zip with dumps>\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	useColor, ok := colorMode(*color)
	if !ok {
		flag.Usage()
		os.Exit(2)
	}
	in := flag.Arg(0)
	if *pass == "" {
		*pass = os.Getenv("PMM_DUMP_PASS")
	}
	if dump.IsZip(in) {
		os.Exit(runZip(in, *pass, !*quick, *skipQAN, *workers, *open, useColor))
	}
	t0 := time.Now()
	r, err := inspect.Scan(in, *pass, !*quick, *skipQAN, *workers)
	if code := scanError(err); code != 0 {
		os.Exit(code)
	}
	if !*open {
		report.WriteText(os.Stdout, r, useColor)
		return
	}
	out := strings.TrimSuffix(in, ".gz")
	out = strings.TrimSuffix(out, ".tar") + ".html"
	if err := writeHTML(r, out, t0); err != nil {
		fail(err)
	}
}

// runZip reports every dump inside a zip, one after the other, and goes on
// when one fails. The exit code is the worst one: 3 for an unknown QAN
// layout, 1 for any other error.
func runZip(in, pass string, deep, skipQAN bool, workers int, open, useColor bool) int {
	entries, err := inspect.ScanZip(in, pass, deep, skipQAN, workers)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pmm-dump-info:", err)
		return 1
	}
	if !open {
		fmt.Printf("\n%s: %d files\n", filepath.Base(in), len(entries))
	}
	worst := 0
	for _, e := range entries {
		t0 := time.Now()
		if e.Err != nil {
			code := scanError(fmt.Errorf("%s > %s: %w", filepath.Base(in), e.Name, e.Err))
			worst = max(worst, code)
			continue
		}
		if !open {
			report.WriteText(os.Stdout, e.Result, useColor)
			continue
		}
		member := strings.TrimSuffix(path.Base(e.Name), ".gz")
		out := strings.TrimSuffix(in, ".zip") + "_" + strings.TrimSuffix(member, ".tar") + ".html"
		if err := writeHTML(e.Result, out, t0); err != nil {
			fmt.Fprintln(os.Stderr, "pmm-dump-info:", err)
			worst = max(worst, 1)
		}
	}
	return worst
}

// scanError prints a scan error and returns its exit code (0 when nil).
func scanError(err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, "pmm-dump-info:", err)
	var qe *inspect.QANSchemaError
	if errors.As(err, &qe) {
		return 3
	}
	return 1
}

// writeHTML writes the HTML report to out and opens it in the browser.
func writeHTML(r *inspect.Result, out string, t0 time.Time) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := report.WriteHTML(f, r); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%d vm chunks, %d services, %d QAN rows in %s -> %s\n",
		len(r.VM.Chunks), len(r.VM.Services), r.QAN.Rows, time.Since(t0).Round(time.Second), out)

	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	return exec.Command(cmd, out).Start()
}

// colorMode resolves -color: auto is on when stdout is a terminal, NO_COLOR is
// unset and TERM is not dumb.
func colorMode(mode string) (on, ok bool) {
	switch mode {
	case "always":
		return true, true
	case "never":
		return false, true
	case "auto":
		fi, err := os.Stdout.Stat()
		return err == nil && fi.Mode()&os.ModeCharDevice != 0 &&
			os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb", true
	}
	return false, false
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "pmm-dump-info:", err)
	os.Exit(1)
}
