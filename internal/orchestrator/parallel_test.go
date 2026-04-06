package orchestrator

import "testing"

func TestLinkOpFields(t *testing.T) {
	op := LinkOp{Src: "/store/ab/abc", Dst: "/vendor/pkg/file.php", Executable: true}
	if op.Src != "/store/ab/abc" || op.Dst != "/vendor/pkg/file.php" || !op.Executable {
		t.Error("LinkOp fields")
	}
}
