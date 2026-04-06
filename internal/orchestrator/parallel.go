package orchestrator

// LinkOp represents a single file link operation for the parallel worker pool.
type LinkOp struct {
	Src        string // CAS file path
	Dst        string // vendor destination path
	Executable bool   // whether the file needs executable permissions
}
