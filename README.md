# pmm-dump-info

Tells you what is inside a [pmm-dump](https://github.com/percona/pmm-dump) file before you import it into a PMM Server.

A dump exported from the PMM UI or with the pmm-dump CLI can be large, and importing it takes time and a matching PMM Server. Often it turns out to be missing what you need (no QAN, no OS metrics, the wrong time range). pmm-dump-info reads the file in place and answers in seconds:

- which PMM and pmm-dump versions made it, from the UI or the CLI, and whether it is encrypted;
- how it was exported (time range, `--instance`, `--ts-selector`, `--where`, QAN or not), with secrets masked;
- which services and nodes have metrics, and for which real time range;
- whether there is QAN, for which services and how many rows;
- what is wrong with the export, as a list of alerts.

```
pmm-dump-info pmm-dump-1792505141.tar.gz          # text report
pmm-dump-info -open pmm-dump-1792505141.tar.gz    # HTML report in the browser
pmm-dump-info -quick big-dump.tar.gz              # versions and export arguments only, faster
```

Nothing is sent anywhere. The HTML report has its data, script and styles inline, so the dump data stays on the machine.

## Install

Go 1.24 or later, standard library only.

```
git clone git@github.com:lulofernandez/pmm-dump-info.git
cd pmm-dump-info
go build -o pmm-dump-info .
```

On macOS, `./build-mac.sh` builds `dist/pmm-dump-info` (universal) and `dist/pmm-dump-info-linux-amd64`. Copy the Linux binary to the server where the dumps are, to read them there without downloading them.

To open dumps from Finder:

```
./install-finder.sh
```

Then right-click a dump > Open With > pmm-dump-info. The HTML report is written next to the dump and opened in the browser. Several files can be selected at once.

## Usage

```
pmm-dump-info [flags] <pmm-dump.tar.gz | .zip with dumps>
  -open          write the HTML report (<dump>.html next to the dump) and open it in the browser
  -quick         only meta.json, log.json and the file list; do not read the chunks
  -j <n>         chunks parsed in parallel (default 4; keep it low on shared hosts)
  -pass <pass>   password of an encrypted dump (also env PMM_DUMP_PASS)
  -skip-qan      do not read the QAN rows
  -color <mode>  auto (default), always (e.g. for less -R), never
```

- **Encrypted dumps** (the CLI default since pmm-dump 0.8.0) need the password pmm-dump printed at export time, with `-pass` or `PMM_DUMP_PASS`.
- **A .zip with several dumps** (common for PMM UI exports) is read without extracting it. You get one report per dump, and a dump that fails does not stop the others.
- **Big dumps.** The full read takes about 40 s for 600 MB. `-quick` skips the chunks, but it still decompresses the whole file, about 1 minute for 3 GB and 2 minutes for 9 GB.

## Example

A CLI export of two MySQL services, with QAN requested:

```text
$ pmm-dump-info pmm-dump-1792505141.tar.gz
PMM-DUMP-INFO  pmm-dump-1792505141.tar.gz (3.5 kB)

Dump
  pmm-dump         pmm-3.7.1@e95ba62
  PMM Server       3.7.1
  Exported from    pmm-dump CLI
  PMM timezone     -
  Encrypted        no
  Requested range  2026-10-20T10:00:00Z -> 2026-10-20T14:00:00Z
  Core metrics     yes (format json)
  QAN              yes
  --instance       db-01-mysql, db-02-mysql

Alerts
  ! QAN was requested (--dump-qan) but the dump has no QAN data (no ch/ files); log.json: ClickHouse returned 0 rows for the filter and period
  ! No OS (node) metrics for node db-01: --instance names its service db-01-mysql but not the node
  ! mysql db-02-mysql has no samples in 1 of its 4 chunks (gaps in collection)
  i log.json has 1 warning lines (see Export log)

Metrics (vm/): 4 chunks, 6.7 kB, chunks cover 2026-10-20 10:00:00Z -> 2026-10-20 14:00:00Z
  real data 2026-10-20 10:00:00Z -> 2026-10-20 13:59:50Z, 18.4k samples, 14 distinct series

  SERVICE      TYPE   NODE   FIRST                 LAST                  SERIES  SAMPLES  METRIC FAMILIES
  db-02        node   db-02  2026-10-20 10:00:00Z  2026-10-20 13:59:50Z  4       5760     node 3, up 1
  db-01-mysql  mysql  db-01  2026-10-20 10:00:00Z  2026-10-20 13:59:50Z  5       7200     mysql 3, scrape 1, up 1
  db-02-mysql  mysql  db-02  2026-10-20 10:00:00Z  2026-10-20 13:59:50Z  5       5400     mysql 3, scrape 1, up 1

QAN (ch/): 0 files, 0 B

Export command:
  export --pmm-url=https://admin:***@pmm.example.com --start-ts=2026-10-20T10:00:00Z --end-ts=2026-10-20T14:00:00Z --instance=db-01-mysql,db-02-mysql --dump-core=true --dump-qan=true --dump-path=dump.tar.gz

Export log (log.json): 4 lines, 2026-10-20T14:05:11Z -> 2026-10-20T14:05:41Z (30s), levels warn 1, info 3
  2026-10-20T14:05:12Z WARN Failed to get PMM version error=connection refused
```

Before importing anything, this says the dump has no QAN at all, no CPU, memory or disk metrics for `db-01`, and a one-hour hole in `db-02-mysql`. If you need any of those, export the dump again with the right options.

The HTML report (`-open`) shows the same, plus a coverage chart with metrics and QAN per service over time, and the samples per chunk.

## What it checks

| Area | Checks |
|---|---|
| Time range | `--start-ts`/`--end-ts` not set, so only the 4 hours before the export (pmm-dump default); `--end-ts` after the export time; metrics starting later or ending earlier than requested |
| `--instance` | an instance with no metrics at all; a service named without its node, so no OS metrics for that node; a node named but no node metrics, with a hint when the pmm-dump version matches `--instance` only against `service_name` (PMM 3.0.0 to 3.7.x, 2.x before 2.43.0) |
| `--ts-selector` | `label="a\|b"` (with `=` the `\|` is literal and matches nothing; `=~` was meant) |
| Metrics | core metrics requested but no chunks; chunks with no samples (the filters matched nothing); services with only `up`/`scrape` series (the exporter returned nothing); gaps in a service's collection |
| QAN | requested but missing or empty, with the row count from `log.json`; MySQL, MongoDB and PostgreSQL services with metrics but no QAN; export done without `--dump-qan` |
| Export log | error and warning lines in `log.json` |
| The file | read errors in chunks or QAN files; dump from a PMM newer than the QAN layouts the tool knows |

## Reading the output

| Section | What it tells you |
|---|---|
| Dump | versions, UI or CLI export, encryption, and the export arguments as pmm-dump applied them (defaults included) |
| Alerts | `!` is a problem with the export or the data, `i` is information. Read these first |
| Metrics (vm/) | one row per service and node with type, node, first and last real sample, series, samples and metric families. A service missing here is not in the dump |
| QAN (ch/) | rows, distinct queries and time range per service |
| Export command | the full command line, secrets replaced by `***` |
| Export log | the warning and error lines of `log.json` |

"Requested range" is what the export asked for. The real range of each service is in its FIRST and LAST columns, and that is what you will see after the import.

The report shows only the warnings and errors of `log.json`. To read the whole export log:

```
tar xOzf <dump.tar.gz> log.json | jq -r '"\(.time) \(.level) \(.message)"'
```

## Exit codes

| Code | Meaning |
|---|---|
| 0 | report printed |
| 1 | error (not a pmm-dump file, wrong password, read error) |
| 2 | usage |
| 3 | the QAN rows follow a column layout this build does not know |

With a .zip, the exit code is the worst one of its dumps.

## Limits

- **Native format.** Dumps exported with `--vm-native-data` are not decoded, so services and real time ranges are unknown and only the chunk time ranges are shown.
- **QAN layouts.** QAN rows have no header, and their column order changes between PMM releases. The tool knows the layouts up to the PMM release it was built with, and a dump from a newer PMM may stop it with exit code 3. Then run `tools/update-qan-schema.sh` and rebuild, or use `-skip-qan` for the rest of the report.
- **`-quick`** does not read the chunks, so the checks based on them (services, samples, gaps, real range, `--instance`) are skipped. The report lists which ones.
- **It reads, it does not import.** It does not show metric values or graphs. For that, import the dump into a PMM Server.

## Development

```
go test ./...
tools/update-qan-schema.sh --check   # warns when PMM has new QAN migrations (also run by build-mac.sh)
```

`internal/inspect/qan_columns.txt` is generated from the PMM ClickHouse migrations by `tools/update-qan-schema.sh`. Do not edit it by hand.

Never add real dumps, logs or outputs to the repository, not even as test fixtures. Tests build their dumps in code.
