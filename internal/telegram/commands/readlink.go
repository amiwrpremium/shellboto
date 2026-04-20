package commands

import "os"

// readlink wraps os.Readlink for easy testability later. Kept separate so
// the main ExecShell helpers don't pull os in.
func readlink(name string) (string, error) { return os.Readlink(name) }
