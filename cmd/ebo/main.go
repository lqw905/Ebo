package main

import (
	"os"

	"github.com/lqw905/Ebo/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
