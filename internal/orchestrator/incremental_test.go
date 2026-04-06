package orchestrator

import "testing"

func TestIsNoopHashMatch(t *testing.T) {
	if !IsNoop("sha256:abc", "sha256:abc", true, true) {
		t.Error("same hash + same dev = noop")
	}
}

func TestIsNoopHashDiffers(t *testing.T) {
	if IsNoop("sha256:abc", "sha256:def", true, true) {
		t.Error("different hash = not noop")
	}
}

func TestIsNoopDevDiffers(t *testing.T) {
	if IsNoop("sha256:abc", "sha256:abc", true, false) {
		t.Error("different dev flag = not noop")
	}
}

func TestIsNoopBothDiffer(t *testing.T) {
	if IsNoop("sha256:abc", "sha256:def", true, false) {
		t.Error("both differ = not noop")
	}
}
