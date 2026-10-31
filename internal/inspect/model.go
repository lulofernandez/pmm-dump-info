// Package inspect reads a pmm-dump file in one pass and summarizes what it
// holds: versions, export arguments, services with metrics, QAN, time range.
package inspect

// Result is everything the reports show. Times are Unix milliseconds.
type Result struct {
	File      string `json:"file"`
	Size      int64  `json:"size"`
	Encrypted bool   `json:"encrypted"`
	Deep      bool   `json:"deep"` // chunk contents were read

	Meta   *Meta      `json:"meta"`
	Args   Args       `json:"args"`
	Log    LogSummary `json:"log"`
	VM     VMSummary  `json:"vm"`
	QAN    QANSummary `json:"qan"`
	Other  []string   `json:"other,omitempty"` // tar entries outside meta/log/vm/ch
	Alerts []Alert    `json:"alerts"`
}

// Meta is meta.json as written by pmm-dump (pkg/dump.Meta).
type Meta struct {
	Version struct {
		GitBranch string `json:"git-branch"`
		GitCommit string `json:"git-commit"`
	} `json:"version"`
	PMMServerVersion  string  `json:"pmm-server-version"`
	MaxChunkSize      int64   `json:"max_chunk_size"`
	PMMTimezone       *string `json:"pmm-server-timezone"`
	Arguments         string  `json:"arguments"`
	VMDataFormat      string  `json:"vm-data-format"`
	PMMServerServices []struct {
		Name      string   `json:"name"`
		NodeID    string   `json:"node-id"`
		NodeName  string   `json:"node-name"`
		AgentsIDs []string `json:"agents-ids"`
	} `json:"pmm-server-services,omitempty"`
}

// Args is the export command line from meta.json, parsed.
type Args struct {
	Masked     string   `json:"masked"` // secrets replaced by ***
	Source     string   `json:"source"` // "PMM UI" or "pmm-dump CLI"
	Start      string   `json:"start,omitempty"`
	End        string   `json:"end,omitempty"`
	Instances  []string `json:"instances,omitempty"` // deduplicated
	Dashboards []string `json:"dashboards,omitempty"`
	TSSelector string   `json:"tsSelector,omitempty"`
	Where      string   `json:"where,omitempty"`
	DumpQAN    bool     `json:"dumpQAN"`
	DumpCore   bool     `json:"dumpCore"`
	Flags      []Flag   `json:"flags"`
}

// Flag is one --name=value of the export command line.
type Flag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Count int    `json:"count"` // times repeated with the same value
}

// LogSummary comes from log.json (zerolog JSON lines).
type LogSummary struct {
	Present bool           `json:"present"`
	Lines   int            `json:"lines"`
	Levels  map[string]int `json:"levels"`
	CHRows  *int64         `json:"chRows,omitempty"`  // "Split Click House rows into chunks"
	First   string         `json:"first,omitempty"`   // time of the first line
	Last    string         `json:"last,omitempty"`    // time of the last line
	Notable []string       `json:"notable,omitempty"` // warn/error lines and QAN lines
}

// VMSummary covers the vm/ chunks (VictoriaMetrics export).
type VMSummary struct {
	Format   string    `json:"format"` // json or native
	Chunks   []Chunk   `json:"chunks"` // sorted by start
	Bytes    int64     `json:"bytes"`
	First    int64     `json:"first"` // real sample range (deep scan)
	Last     int64     `json:"last"`
	Samples  int64     `json:"samples"`
	Series   int       `json:"series"` // distinct label sets
	Services []Service `json:"services"`
	Errors   []string  `json:"errors,omitempty"`
}

// Chunk is one vm/<start>-<end>.bin file.
type Chunk struct {
	Name    string `json:"name"`
	Start   int64  `json:"start"` // from the file name
	End     int64  `json:"end"`
	Bytes   int64  `json:"bytes"`
	Lines   int    `json:"lines"`
	Samples int64  `json:"samples"`
}

// Service groups the series of one service_name, or of one node for the
// series without service_name (node_exporter, pmm-agent, vmagent).
type Service struct {
	Name     string         `json:"name"`
	Kind     string         `json:"kind"` // service_type (mysql, mongodb...) or "node"
	Node     string         `json:"node"`
	First    int64          `json:"first"`
	Last     int64          `json:"last"`
	Series   int            `json:"series"`
	Samples  int64          `json:"samples"`
	Families map[string]int `json:"families"` // metric name prefix -> distinct series
	Coverage []int64        `json:"coverage"` // samples per chunk, same order as VMSummary.Chunks
}

// QANSummary covers the ch/ chunks (ClickHouse pmm.metrics rows).
type QANSummary struct {
	Files    []string     `json:"files"`
	Bytes    int64        `json:"bytes"`
	Rows     int64        `json:"rows"`
	Columns  int          `json:"columns"` // fields per row seen
	Schema   string       `json:"schema"`  // how columns were identified
	Skipped  bool         `json:"skipped"` // -skip-qan: rows not read
	First    int64        `json:"first"`
	Last     int64        `json:"last"`
	Services []QANService `json:"services"`
	Errors   []string     `json:"errors,omitempty"`
}

// QANService is the QAN of one service.
type QANService struct {
	Name    string          `json:"name"`
	Kind    string          `json:"kind"`
	Node    string          `json:"node"`
	Rows    int64           `json:"rows"`
	Queries int             `json:"queries"` // distinct queryid
	First   int64           `json:"first"`
	Last    int64           `json:"last"`
	Buckets map[int64]int64 `json:"buckets"` // period_start -> rows
}

// Alert is a finding shown at the top of the report.
type Alert struct {
	Level string `json:"level"` // warn or info
	Text  string `json:"text"`
}
