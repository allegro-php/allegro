package cli

import "testing"

func TestExitCodeValues(t *testing.T) {
	if ExitSuccess != 0 {
		t.Errorf("ExitSuccess = %d, want 0", ExitSuccess)
	}
	if ExitGeneralError != 1 {
		t.Errorf("ExitGeneralError = %d, want 1", ExitGeneralError)
	}
	if ExitProjectFile != 2 {
		t.Errorf("ExitProjectFile = %d, want 2", ExitProjectFile)
	}
	if ExitNetworkError != 3 {
		t.Errorf("ExitNetworkError = %d, want 3", ExitNetworkError)
	}
	if ExitFilesystemError != 4 {
		t.Errorf("ExitFilesystemError = %d, want 4", ExitFilesystemError)
	}
	if ExitComposerError != 5 {
		t.Errorf("ExitComposerError = %d, want 5", ExitComposerError)
	}
}
