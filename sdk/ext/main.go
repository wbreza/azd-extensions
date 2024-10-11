package ext

import (
	"io"
	"log"
	"os"

	azcorelog "github.com/Azure/azure-sdk-for-go/sdk/azcore/log"

	"github.com/spf13/pflag"
)

func init() {
	if isDebugEnabled() {
		azcorelog.SetListener(func(event azcorelog.Event, msg string) {
			log.Printf("%s: %s\n", event, msg)
		})
	} else {
		log.SetOutput(io.Discard)
	}
}

// isDebugEnabled checks to see if `--debug` was passed with a truthy
// value.
func isDebugEnabled() bool {
	debug := false
	flags := pflag.NewFlagSet("", pflag.ContinueOnError)

	// Since we are running this parse logic on the full command line, there may be additional flags
	// which we have not defined in our flag set (but would be defined by whatever command we end up
	// running). Setting UnknownFlags instructs `flags.Parse` to continue parsing the command line
	// even if a flag is not in the flag set (instead of just returning an error saying the flag was not
	// found).
	flags.ParseErrorsWhitelist.UnknownFlags = true
	flags.BoolVar(&debug, "debug", false, "")

	// if flag `-h` of `--help` is within the command, the usage is automatically shown.
	// Setting `Usage` to a no-op will hide this extra unwanted output.
	flags.Usage = func() {}

	_ = flags.Parse(os.Args[1:])
	return debug
}
