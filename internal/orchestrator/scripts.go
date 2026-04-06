package orchestrator

// ComposerRunner abstracts Composer subprocess calls for testability.
type ComposerRunner interface {
	Run(args ...string) error
}
