package inspect

import (
	"regexp"
	"strings"
)

// secretFlags hold credentials; their values are never shown.
var secretFlags = map[string]bool{
	"pmm-cookie": true, "pmm-pass": true, "pmm-token": true, "pass": true,
	"encryption-password": true,
}

var urlUserinfo = regexp.MustCompile(`([a-z]+://[^:/@\s]+):([^@\s]+)@`)

// ParseArgs parses the "arguments" string of meta.json, e.g.
// "export --pmm-url=http://... --dump-qan=true --instance=a,b".
// pmm-dump joins os.Args with spaces, so a token that does not start with
// "-" belongs to the value of the previous flag (--where="a = 'b'").
func ParseArgs(s string) Args {
	var a Args
	type kv struct{ name, value string }
	var flags []kv
	for _, tok := range strings.Fields(s) {
		switch {
		case strings.HasPrefix(tok, "-"):
			name, value, ok := strings.Cut(strings.TrimLeft(tok, "-"), "=")
			if !ok {
				value = "true"
			}
			flags = append(flags, kv{name, value})
		case len(flags) > 0:
			flags[len(flags)-1].value += " " + tok
		}
	}

	seen := map[string]int{} // name=value -> index in a.Flags
	var masked []string
	if len(s) > 0 {
		if cmd, _, _ := strings.Cut(s, " "); !strings.HasPrefix(cmd, "-") {
			masked = append(masked, cmd)
		}
	}
	for _, f := range flags {
		v := f.value
		key := ""
		switch {
		case secretFlags[f.name]:
			v = "***"
		case f.name == "instance" || f.name == "instances" || f.name == "dashboard" || f.name == "dashboards":
			// the PMM UI repeats the whole list inside and across flags:
			// one entry per flag name with the union of the values
			key = f.name
		default:
			v = maskURL(v)
		}
		if key == "" {
			key = f.name + "=" + v
		}
		if i, ok := seen[key]; ok {
			a.Flags[i].Count++
			if key == f.name {
				a.Flags[i].Value = strings.Join(appendUnique(strings.Split(a.Flags[i].Value, ","), strings.Split(v, ",")...), ",")
			}
		} else {
			if key == f.name {
				v = strings.Join(appendUnique(nil, strings.Split(v, ",")...), ",")
			}
			seen[key] = len(a.Flags)
			a.Flags = append(a.Flags, Flag{Name: f.name, Value: v, Count: 1})
		}

		val := strings.Trim(f.value, `"'`)
		switch f.name {
		case "start-ts":
			a.Start = val
		case "end-ts":
			a.End = val
		case "instance", "instances":
			a.Instances = appendUnique(a.Instances, strings.Split(val, ",")...)
		case "dashboard", "dashboards":
			a.Dashboards = appendUnique(a.Dashboards, strings.Split(val, ",")...)
		case "ts-selector":
			a.TSSelector = val
		case "where", "w":
			a.Where = val
		case "dump-qan":
			a.DumpQAN = val != "false"
		case "no-dump-qan":
			a.DumpQAN = false
		case "dump-core":
			a.DumpCore = val != "false"
		case "no-dump-core":
			a.DumpCore = false
		}
	}
	// pmm-dump exports core metrics unless --no-dump-core / --dump-core=false.
	if !hasFlag(a.Flags, "dump-core") && !hasFlag(a.Flags, "no-dump-core") {
		a.DumpCore = true
	}
	for _, f := range a.Flags {
		masked = append(masked, "--"+f.Name+"="+f.Value)
	}
	a.Masked = strings.Join(masked, " ")

	a.Source = "pmm-dump CLI"
	for _, f := range a.Flags {
		if f.Name == "pmm-cookie" || (f.Name == "dump-path" && strings.HasPrefix(f.Value, "/srv/dump/")) {
			a.Source = "PMM UI (Help > PMM Dump)"
		}
	}
	return a
}

// maskURL hides the password of user:password@ in URLs (--pmm-url,
// --click-house-url).
func maskURL(v string) string {
	return urlUserinfo.ReplaceAllString(v, "$1:***@")
}

func hasFlag(fs []Flag, name string) bool {
	for _, f := range fs {
		if f.Name == name {
			return true
		}
	}
	return false
}

func appendUnique(list []string, vals ...string) []string {
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		dup := false
		for _, x := range list {
			if x == v {
				dup = true
				break
			}
		}
		if !dup {
			list = append(list, v)
		}
	}
	return list
}
