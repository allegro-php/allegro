package config

import (
	"encoding/json"
	"testing"
)

func TestConfigStructFields(t *testing.T) {
	c := Config{
		StorePath:      "/custom/store",
		Workers:        16,
		LinkStrategy:   "reflink",
		NoProgress:     true,
		NoColor:        false,
		ComposerPath:   "/usr/bin/composer",
		NoDev:          true,
		NoScripts:      false,
		PruneStaleDay:  90,
	}
	if c.StorePath != "/custom/store" { t.Error("StorePath") }
	if c.Workers != 16 { t.Error("Workers") }
	if c.LinkStrategy != "reflink" { t.Error("LinkStrategy") }
	if !c.NoProgress { t.Error("NoProgress") }
	if c.NoColor { t.Error("NoColor") }
	if c.ComposerPath != "/usr/bin/composer" { t.Error("ComposerPath") }
	if !c.NoDev { t.Error("NoDev") }
	if c.NoScripts { t.Error("NoScripts") }
	if c.PruneStaleDay != 90 { t.Error("PruneStaleDay") }
}

func TestConfigJSONRoundtrip(t *testing.T) {
	c := Config{Workers: 8, LinkStrategy: "auto", PruneStaleDay: 90}
	data, err := json.Marshal(c)
	if err != nil { t.Fatal(err) }
	var c2 Config
	if err := json.Unmarshal(data, &c2); err != nil { t.Fatal(err) }
	if c2.Workers != 8 || c2.PruneStaleDay != 90 {
		t.Errorf("roundtrip: %+v", c2)
	}
}
