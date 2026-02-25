package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(exitGeneric)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	code, err := runCommand(cmd, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	os.Exit(code)
}
