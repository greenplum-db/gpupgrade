/*
Package file simply exposes our filename constants for upgrade status
monitoring.
*/
package file

const (
	InProgress = "in.progress" // started, but not yet completed or failed
	Complete   = "completed"   // finished successfully
	Failed     = "failed"      // stopped with an error
)
