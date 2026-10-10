// Command servly is the restaurant order management server and its operator
// commands. Run `servly --help`.
package main

import (
	"os"
	_ "time/tzdata" // business dates need IANA zones even on hosts without tzdata

	"github.com/servly-io/servly/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
