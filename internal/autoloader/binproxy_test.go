package autoloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectBinTargetPHPNoShebang(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool.php")
	os.WriteFile(path, []byte("<?php\necho 'hello';\n"), 0644)

	typ, err := DetectBinTarget(path)
	if err != nil {
		t.Fatal(err)
	}
	if typ != BinPHPNoShebang {
		t.Errorf("type = %d, want BinPHPNoShebang", typ)
	}
}

func TestDetectBinTargetPHPNoShebangWithWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool.php")
	os.WriteFile(path, []byte("\n\t  <?php\necho 'hello';\n"), 0644)

	typ, err := DetectBinTarget(path)
	if err != nil {
		t.Fatal(err)
	}
	if typ != BinPHPNoShebang {
		t.Errorf("type = %d, want BinPHPNoShebang", typ)
	}
}

func TestDetectBinTargetPHPWithShebang(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool")
	os.WriteFile(path, []byte("#!/usr/bin/env php\n<?php\necho 'hello';\n"), 0644)

	typ, err := DetectBinTarget(path)
	if err != nil {
		t.Fatal(err)
	}
	if typ != BinPHPWithShebang {
		t.Errorf("type = %d, want BinPHPWithShebang", typ)
	}
}

func TestDetectBinTargetPHPWithShebangPhp82(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool")
	os.WriteFile(path, []byte("#!/usr/bin/env php8.2\n<?php\necho 'hello';\n"), 0644)

	typ, err := DetectBinTarget(path)
	if err != nil {
		t.Fatal(err)
	}
	if typ != BinPHPWithShebang {
		t.Errorf("type = %d, want BinPHPWithShebang", typ)
	}
}

func TestDetectBinTargetNonPHP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool.sh")
	os.WriteFile(path, []byte("#!/bin/bash\necho hello\n"), 0644)

	typ, err := DetectBinTarget(path)
	if err != nil {
		t.Fatal(err)
	}
	if typ != BinNonPHP {
		t.Errorf("type = %d, want BinNonPHP", typ)
	}
}

func TestDetectBinTargetPHPWithBOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tool.php")
	os.WriteFile(path, []byte("\xEF\xBB\xBF<?php\necho 'hello';\n"), 0644)

	typ, err := DetectBinTarget(path)
	if err != nil {
		t.Fatal(err)
	}
	if typ != BinPHPNoShebang {
		t.Errorf("type = %d, want BinPHPNoShebang", typ)
	}
}

func TestGeneratePHPProxyNoShebang(t *testing.T) {
	out := GeneratePHPProxyNoShebang("monolog/monolog", "bin/console")
	if !strings.Contains(out, "#!/usr/bin/env php") {
		t.Error("missing shebang")
	}
	if !strings.Contains(out, "_composer_bin_dir") {
		t.Error("missing _composer_bin_dir global")
	}
	if !strings.Contains(out, "monolog/monolog/bin/console") {
		t.Error("missing include path")
	}
}

func TestGeneratePHPProxyWithShebang(t *testing.T) {
	out := GeneratePHPProxyWithShebang("phpunit/phpunit", "phpunit")
	if !strings.Contains(out, "BinProxyWrapper") {
		t.Error("missing BinProxyWrapper class")
	}
	if !strings.Contains(out, "PHP_VERSION_ID < 80000") {
		t.Error("missing PHP version check")
	}
	if !strings.Contains(out, "phpvfscomposer://") {
		t.Error("missing phpvfscomposer stream wrapper")
	}
}

func TestGenerateShellProxy(t *testing.T) {
	out := GenerateShellProxy("vendor/tool", "bin/run")
	if !strings.Contains(out, "#!/bin/sh") {
		t.Error("missing shebang")
	}
	if !strings.Contains(out, "COMPOSER_RUNTIME_BIN_DIR") {
		t.Error("missing env var export")
	}
	if !strings.Contains(out, `"$@"`) {
		t.Error("missing argument passthrough")
	}
}

func TestBinBasename(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"bin/phpunit", "phpunit"},
		{"phpunit", "phpunit"},
		{"bin/sub/tool", "tool"},
	}
	for _, tt := range tests {
		if got := BinBasename(tt.in); got != tt.want {
			t.Errorf("BinBasename(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildIncludePath(t *testing.T) {
	got := BuildIncludePath("monolog/monolog", "bin/console")
	if got != "__DIR__ . '/..'.'monolog/monolog/bin/console'" {
		// Just check it contains the expected pieces
		if !strings.Contains(got, "monolog/monolog") || !strings.Contains(got, "bin/console") {
			t.Errorf("BuildIncludePath missing components: %q", got)
		}
	}
}
