// pmm-dump-inspect reads a pmm-dump file without importing it into PMM and
// reports versions, export arguments, services with metrics and QAN, and the
// time range covered.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"pmm-dump-inspect/internal/inspect"
	"pmm-dump-inspect/internal/report"
)

func main() {
	html := flag.Bool("html", false, "write the HTML report (default <dump>.html next to the dump) instead of text")
	out := flag.String("o", "", "output HTML file (implies -html)")
	open := flag.Bool("open", false, "write the HTML report and open it (implies -html)")
	quick := flag.Bool("quick", false, "only meta.json, log.json and the file list; do not read chunk contents")
	pass := flag.String("pass", "", "password of an encrypted dump (also env PMM_DUMP_PASS)")
	workers := flag.Int("j", 4, "chunks parsed in parallel (keep it low on shared hosts)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: pmm-dump-inspect [flags] <pmm-dump.tar.gz>\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	in := flag.Arg(0)
	if *pass == "" {
		*pass = os.Getenv("PMM_DUMP_PASS")
	}
	if *out != "" || *open {
		*html = true
	}

	t0 := time.Now()
	r, err := inspect.Scan(in, *pass, !*quick, *workers)
	if err != nil {
		fail(err)
	}

	if !*html {
		report.WriteText(os.Stdout, r)
		return
	}
	if *out == "" {
		*out = strings.TrimSuffix(in, ".gz")
		*out = strings.TrimSuffix(*out, ".tar") + ".html"
	}
	f, err := os.Create(*out)
	if err != nil {
		fail(err)
	}
	if err := report.WriteHTML(f, r); err != nil {
		f.Close()
		fail(err)
	}
	if err := f.Close(); err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stderr, "%d vm chunks, %d services, %d QAN rows in %s -> %s\n",
		len(r.VM.Chunks), len(r.VM.Services), r.QAN.Rows, time.Since(t0).Round(time.Second), *out)

	if *open {
		cmd := "xdg-open"
		if runtime.GOOS == "darwin" {
			cmd = "open"
		}
		if err := exec.Command(cmd, *out).Start(); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "pmm-dump-inspect:", err)
	os.Exit(1)
}
