// Command gh-tree is a gh extension that shows GitHub issues, pull requests,
// and branches as a tree.
package main

import (
	"os"

	"github.com/ohzw/gh-tree/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
