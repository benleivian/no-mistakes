package main

import "os"

// Fork-only (see FORK.md): FAKE_CLI_MODE=ddev serves `ddev list --json-output`
// from FAKE_DDEV_LIST and accepts `ddev delete -Oy <name>`.
func fakeDDEVHandler(args []string) {
	if len(args) == 2 && args[0] == "list" && args[1] == "--json-output" {
		data, err := os.ReadFile(os.Getenv("FAKE_DDEV_LIST"))
		if err != nil {
			os.Exit(1)
		}
		_, _ = os.Stdout.Write(data)
		return
	}
	if len(args) == 3 && args[0] == "delete" && args[1] == "-Oy" {
		return
	}
	os.Exit(1)
}
