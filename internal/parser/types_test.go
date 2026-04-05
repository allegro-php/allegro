package parser

import (
	"encoding/json"
	"testing"
)

func TestPackageJSONRoundTrip(t *testing.T) {
	raw := `{
		"name": "monolog/monolog",
		"version": "3.9.0",
		"version_normalized": "3.9.0.0",
		"type": "library",
		"dist": {
			"type": "zip",
			"url": "https://example.com/monolog.zip",
			"reference": "abc123",
			"shasum": ""
		},
		"autoload": {
			"psr-4": {"Monolog\\": "src/"}
		},
		"bin": ["bin/console"],
		"description": "Logging library"
	}`

	var pkg Package
	if err := json.Unmarshal([]byte(raw), &pkg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pkg.Name != "monolog/monolog" {
		t.Errorf("name = %q, want monolog/monolog", pkg.Name)
	}
	if pkg.Dist == nil || pkg.Dist.Type != "zip" {
		t.Error("dist not parsed correctly")
	}
	if len(pkg.Bin) != 1 || pkg.Bin[0] != "bin/console" {
		t.Errorf("bin = %v, want [bin/console]", pkg.Bin)
	}

	out, err := json.Marshal(pkg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var roundtrip Package
	if err := json.Unmarshal(out, &roundtrip); err != nil {
		t.Fatalf("unmarshal roundtrip: %v", err)
	}
	if roundtrip.Name != pkg.Name {
		t.Error("roundtrip name mismatch")
	}
}

func TestDistEmptyShasum(t *testing.T) {
	raw := `{"type":"zip","url":"https://x.com/a.zip","reference":"abc","shasum":""}`
	var d Dist
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Shasum != "" {
		t.Errorf("shasum = %q, want empty", d.Shasum)
	}
}

func TestComposerLockStruct(t *testing.T) {
	raw := `{
		"packages": [{"name":"a/b","version":"1.0.0"}],
		"packages-dev": [{"name":"c/d","version":"2.0.0"}],
		"content-hash": "abc123"
	}`
	var lock ComposerLock
	if err := json.Unmarshal([]byte(raw), &lock); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(lock.Packages) != 1 {
		t.Errorf("packages len = %d, want 1", len(lock.Packages))
	}
	if len(lock.PackagesDev) != 1 {
		t.Errorf("packages-dev len = %d, want 1", len(lock.PackagesDev))
	}
}
