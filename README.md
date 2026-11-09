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

Before importing anything, this says the dump has no QAN at all, no CPU, memory or disk metrics for `db-01`, and a one-hour hole in `db-02-mysql`.

The HTML report (`-open`) shows the same, plus a coverage chart with metrics and QAN per service over time, and the samples per chunk.

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

## Exit codes

| Code | Meaning |
|---|---|
| 0 | the report was printed. Alerts do not change the exit code |
| 1 | the file could not be read: not a pmm-dump file, missing or wrong password, a damaged file |
| 2 | wrong command line: no file or more than one given, or an invalid `-color` value. The usage help is printed |
| 3 | the QAN rows follow a column layout this build does not know (see Keeping it up to date) |

A .zip with several dumps gives one exit code for all of them. If every dump was read, it is 0. If any dump failed, it is 3 when one of them hit an unknown QAN layout, otherwise 1. The dumps that were read still get their report.

## Keeping it up to date

The QAN rows in a dump have no header. Their column order is the one of the ClickHouse table `pmm.metrics` in the PMM that exported the dump, and PMM releases change it. The known layouts are in `internal/inspect/qan_columns.txt` and are updated in this repository when PMM changes the table.

To get the latest version:

```
git pull
go build -o pmm-dump-info .
```

The tool tells you when it may be out of date:

- the report warns when the dump comes from a PMM newer than the latest release the layouts know;
- the tool stops with exit code 3 when the QAN rows follow no known layout, and says which file and row did not match.

Until a version with the new layout is available, `-skip-qan` gives the rest of the report.

## Limits

- **Native format.** Dumps exported with `--vm-native-data` are not decoded, so services and real time ranges are unknown and only the chunk time ranges are shown.
- **QAN layouts.** QAN rows have no header, and their column order changes between PMM releases. The tool knows the layouts up to the PMM release it was built with, and a dump from a newer PMM may stop it with exit code 3 (see Keeping it up to date).
- **`-quick`** does not read the chunks, so the checks based on them (services, samples, gaps, real range, `--instance`) are skipped. The report lists which ones.
- **It reads, it does not import.** It does not show metric values or graphs. For that, import the dump into a PMM Server.

## Development

Run the unit tests after a code change:

```
go test ./...
```

They check, without real dumps, the parsing of the export arguments (including secret masking), each alert, the QAN row checks against the known layouts, reading dumps inside a .zip, and the table alignment of the colored text report. All packages should print `ok`.

`qan_columns.txt` is generated from the PMM ClickHouse migrations (`qan-api2/migrations/sql` in percona/pmm, and percona/qan-api2 for releases older than PMM 2.33). Do not edit it by hand. To check for new migrations and regenerate it (needs git, Go and access to github.com):

```
tools/update-qan-schema.sh --check   # exit 1 when PMM has new migrations
tools/update-qan-schema.sh           # rewrite qan_columns.txt
```

If the script fails, a new migration uses SQL that `tools/qanschema` does not parse yet, or a column type that `internal/qantype` cannot check, and the code needs a change.

Never add real dumps, logs or outputs to the repository, not even as test fixtures. Tests build their dumps in code.
