package cmd

func ShellCommandArgs(command string) []string {
	return []string{"sh", "-lc", command}
}
