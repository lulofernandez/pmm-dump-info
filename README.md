# pmm-dump-info

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
- QAN: see below.

## QAN columns

The `ch/*.tsv` files have no header: the column order is the one of the ClickHouse table `pmm.metrics` in the exporting PMM, which changes between PMM releases. `internal/inspect/qan_columns.txt` holds that order (and the column types) after each PMM release that changed it. It is generated from the ClickHouse migrations of PMM (`qan-api2/migrations/sql` in percona/pmm, and percona/qan-api2 for the ones older than PMM 2.33) by `tools/update-qan-schema.sh`; do not edit it by hand.

Every QAN row is checked against the column types (numbers, dates, enum values, arrays), and the layout used is the one of the PMM version in `meta.json`. If any row does not match, the tool **stops** (exit code 3) with the file, row and column that failed: the dump comes from a PMM whose `pmm.metrics` layout this build does not know. Then:

```
tools/update-qan-schema.sh     # regenerate qan_columns.txt from the PMM repo
./build-mac.sh                 # rebuild (copy the linux binary to the lab hosts again)
```

Meanwhile `-skip-qan` gives the rest of the report. If the generator itself fails, a new migration uses SQL it does not understand yet (`tools/qanschema`), or a column type the tool cannot check (`internal/qantype`).

Version checks:

- `./build-mac.sh` runs `tools/update-qan-schema.sh --check` and warns when PMM has new migrations (`SKIP_QAN_CHECK=1` to skip it offline);
- the report warns when a dump comes from a PMM newer than the latest release in `qan_columns.txt`, and reminds to run the check when the file was last checked more than 30 days ago.

## Build

```
./build-mac.sh
```

Builds `dist/pmm-dump-info` (macOS universal) and `dist/pmm-dump-info-linux-amd64` (copy it to the lab hosts). Plain `go build -o pmm-dump-info .` works too (Go 1.24 or later, standard library only).

## Finder (macOS)

```
./install-finder.sh
```

Installs `~/Applications/pmm-dump-info.app`. Right-click a dump > Open With > pmm-dump-info: the HTML report is written next to the dump and opened in the browser. Several files can be selected at once. Errors are shown in a dialog. The app runs `dist/pmm-dump-info` from this repo, so after a change `./build-mac.sh` is enough.

## Use

```
pmm-dump-info [flags] <pmm-dump.tar.gz>
  -open          write the HTML report (<dump>.html next to the dump) and open it in the browser
  -quick         only meta.json, log.json and the file list; do not read chunk contents
  -j <n>         chunks parsed in parallel (default 4; keep it low on shared hosts)
  -pass <pass>   password of an encrypted dump (also env PMM_DUMP_PASS)
  -skip-qan      do not read the QAN rows (ch/), e.g. when their column layout is unknown
```

Exit codes: 0 OK, 1 error, 2 usage, 3 unknown QAN column layout (see QAN columns).

Without flags it prints the text report, the one to use on the lab hosts. `-open` is for the Mac (the Finder app uses it). `-quick` answers versions, arguments and "is there QAN" in seconds; the full read of a 600 MB dump takes about 20 s with `-j 4`.
