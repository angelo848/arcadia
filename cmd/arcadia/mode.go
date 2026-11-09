package main

// isAgentMode reports whether args (os.Args[1:]) select agent mode: some arg is
// exactly "--agent" or "-agent" (EXE-001). "--agent=false" and look-alikes such
// as "--agents" select the UI.
func isAgentMode(args []string) bool {
	for _, a := range args {
		if a == "--agent" || a == "-agent" {
			return true
		}
	}
	return false
}
