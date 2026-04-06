package orchestrator

import "testing"

type mockRunner struct {
	called []string
	err    error
}

func (m *mockRunner) Run(args ...string) error {
	m.called = append(m.called, args...)
	return m.err
}

func TestComposerRunnerInterface(t *testing.T) {
	var r ComposerRunner = &mockRunner{}
	if err := r.Run("dumpautoload", "--optimize"); err != nil {
		t.Fatal(err)
	}
}
