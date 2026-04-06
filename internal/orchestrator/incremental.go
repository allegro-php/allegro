package orchestrator

// IsNoop returns true when vendor is already up to date (no changes needed).
// Both lock hash and dev flag must match for a noop.
func IsNoop(stateLockHash, currentLockHash string, stateDev, currentDev bool) bool {
	return stateLockHash == currentLockHash && stateDev == currentDev
}
