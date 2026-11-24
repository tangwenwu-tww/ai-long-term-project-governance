package main

import (
	"os"

	"example.invalid/aigov/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
