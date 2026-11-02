package report

import (
	"regexp"
	"strings"
	"testing"
)

func TestTableColorsKeepAlignment(t *testing.T) {
	rows := [][]string{{"SERVICE", "TYPE", "ROWS"}, {"node1-mysql", "mysql", "1754"}, {"pg", "postgresql", "3"}}
	var plain, colored strings.Builder
	table(&plain, palette{false}, true, false, rows)
	table(&colored, palette{true}, true, false, rows)
	if !strings.Contains(colored.String(), "\x1b[36mSERVICE\x1b[0m") {
		t.Fatalf("header not styled:\n%q", colored.String())
	}
	stripped := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(colored.String(), "")
	if stripped != plain.String() {
		t.Fatalf("colors change the layout:\n%s\nvs\n%s", stripped, plain.String())
	}
	want := "  SERVICE      TYPE        ROWS\n  node1-mysql  mysql       1754\n  pg           postgresql  3\n"
	if plain.String() != want {
		t.Fatalf("got\n%q\nwant\n%q", plain.String(), want)
	}
}
