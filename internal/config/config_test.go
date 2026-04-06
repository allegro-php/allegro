package config

import (
	"encoding/json"
	"os"
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

func TestReadConfigMissing(t *testing.T) {
	c, err := ReadConfig("/nonexistent/config.json")
	if err != nil { t.Fatal(err) }
	// Missing file returns zero-value config (all defaults)
	if c.Workers != 0 { t.Errorf("workers = %d, want 0", c.Workers) }
}

func TestWriteAndReadConfig(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	c := Config{Workers: 16, LinkStrategy: "copy", PruneStaleDay: 30}
	if err := WriteConfig(path, c); err != nil { t.Fatal(err) }
	c2, err := ReadConfig(path)
	if err != nil { t.Fatal(err) }
	if c2.Workers != 16 { t.Errorf("workers = %d", c2.Workers) }
	if c2.LinkStrategy != "copy" { t.Errorf("link_strategy = %q", c2.LinkStrategy) }
	if c2.PruneStaleDay != 30 { t.Errorf("prune_stale_days = %d", c2.PruneStaleDay) }
}

func TestReadConfigMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	os.WriteFile(path, []byte("{bad"), 0644)
	c, err := ReadConfig(path)
	if err != nil { t.Fatal(err) }
	// Malformed returns zero-value config (spec §7.6: warn, use defaults)
	if c.Workers != 0 { t.Error("malformed should return defaults") }
}

func TestResolveWorkersPrecedence(t *testing.T) {
	// Config value
	c := Config{Workers: 12}
	got := ResolveWithConfig(0, "", c.Workers, 8) // no flag, no env, config=12, default=8
	if got != 12 { t.Errorf("config should win over default: %d", got) }

	// Env overrides config
	got = ResolveWithConfig(0, "16", c.Workers, 8)
	if got != 16 { t.Errorf("env should win over config: %d", got) }

	// Flag overrides env
	got = ResolveWithConfig(4, "16", c.Workers, 8)
	if got != 4 { t.Errorf("flag should win over env: %d", got) }

	// Default when nothing set
	got = ResolveWithConfig(0, "", 0, 8)
	if got != 8 { t.Errorf("default should apply: %d", got) }
}
