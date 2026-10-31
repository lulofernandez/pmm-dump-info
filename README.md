# pmm-dump-inspect

Reads a [pmm-dump](https://github.com/percona/pmm-dump) file (the `.tar.gz` a customer exports from PMM) without importing it into a PMM Server, and reports:

- pmm-dump and PMM Server versions, whether it was exported from the PMM UI or the CLI, encryption;
- the export arguments: requested range, `--instance`, `--dashboard`, `--ts-selector`, `--where`, `--dump-qan` (secrets masked, repeated values once);
- the services with metrics: type, node, real first and last sample, distinct series, samples, metric families;
- QAN: rows, distinct queries and time range per service;
- alerts: QAN requested but empty, data shorter than the requested range, `--instance` without metrics, gaps, errors in `log.json`.

The HTML report adds a coverage chart (metrics and QAN per service over time) and the samples per chunk. It has no external requests: data, script and styles are inline, so customer data stays on the machine.

## What a pmm-dump holds

From the pmm-dump source (`pkg/dump`, `pkg/transferer`, `pkg/victoriametrics`, `pkg/clickhouse`):

| Entry | Content |
|---|---|
| `meta.json` | versions, `arguments` (the export command line), `vm-data-format`, optional `pmm-server-services` |
| `log.json` | the export log (zerolog JSON lines); says how many ClickHouse rows were exported |
| `vm/<start>-<end>.bin` | metrics, one chunk per time window: VictoriaMetrics `/api/v1/export` JSON lines (gzip), or the native binary format with `--vm-native-data` |
| `ch/<n>.tsv` | QAN: `SELECT * FROM metrics` of the ClickHouse `pmm` database, tab separated, no header |

With `--encryption` (default since pmm-dump 0.8.0 on the CLI) the whole file is AES-256-CTR, openssl compatible (`Salted__` header, PBKDF2-SHA256, 10000 iterations): pass the password printed at export time with `-pass` or `PMM_DUMP_PASS`.

Limits:

- native format (`--vm-native-data`): the binary chunks are not decoded, only the chunk time range is shown;
- QAN: the `.tsv` has no header and the column order depends on the PMM version; columns are identified with the schemas in `internal/inspect/qan_columns.txt` (matched by column count). With an unknown schema only rows and time range are shown.

## Build

```
./build-mac.sh
```

Builds `dist/pmm-dump-inspect` (macOS universal) and `dist/pmm-dump-inspect-linux-amd64` (copy it to the lab hosts). Plain `go build -o pmm-dump-inspect .` works too (Go 1.24 or later, standard library only).

## Finder (macOS)

```
./install-finder.sh
```

Installs `~/Applications/pmm-dump-inspect.app`. Right-click a dump > Open With > pmm-dump-inspect: the HTML report is written next to the dump and opened in the browser. Several files can be selected at once. Errors are shown in a dialog. The app runs `dist/pmm-dump-inspect` from this repo, so after a change `./build-mac.sh` is enough.

## Use

```
pmm-dump-inspect [flags] <pmm-dump.tar.gz>
  -html          write the HTML report (<dump>.html next to the dump) instead of text
  -o <file>      HTML output file (implies -html)
  -open          write the HTML report and open it (implies -html)
  -quick         only meta.json, log.json and the file list; do not read chunk contents
  -j <n>         chunks parsed in parallel (default 4; keep it low on shared hosts)
  -pass <pass>   password of an encrypted dump (also env PMM_DUMP_PASS)
```

Without flags it prints the text report. `-quick` answers versions, arguments and "is there QAN" in seconds; the full read of a 600 MB dump takes about 20 s with `-j 4`.
