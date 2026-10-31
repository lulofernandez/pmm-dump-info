package main

import "testing"

func TestApply(t *testing.T) {
	cols, err := apply(nil, "CREATE TABLE metrics (\n"+
		"  -- dims, with a comma\n"+
		"  `a` LowCardinality(String) COMMENT 'x, y',\n"+
		"  `e` Enum8('p' = 0, 'q' = 1) COMMENT 'enum',\n"+
		"  `old` Float32,\n"+
		"  `m` Float32 MATERIALIZED 1,\n"+
		"  `z` UInt32\n"+
		") ENGINE = {{ .engine }} ORDER BY a;\n"+
		"ALTER TABLE metrics ADD COLUMN `b` String AFTER `a`, ADD COLUMN c Array(String);\n"+
		"ALTER TABLE metrics RENAME COLUMN old to new, DROP COLUMN `z`;\n"+
		"ALTER TABLE metrics MODIFY COLUMN `e` Enum8('p'=0,'q'=1,'r'=2) COMMENT 'more';")
	if err != nil {
		t.Fatal(err)
	}
	want := []column{{"a", "LowCardinality(String)"}, {"b", "String"}, {"e", "Enum8('p'=0,'q'=1,'r'=2)"}, {"new", "Float32"}, {"c", "Array(String)"}}
	if !same(cols, want) {
		t.Errorf("got %v", cols)
	}
}

func TestApplyUnknown(t *testing.T) {
	if _, err := apply([]column{{"a", "String"}}, "ALTER TABLE metrics MOVE PARTITION x TO DISK 'd';"); err == nil {
		t.Error("an unknown ALTER on metrics must fail")
	}
}
