package version

import "fmt"

// we can't have those variables filled by the `-ldflags="-X ..."` in the `cmd/manager` package because
// it's imported as `main`

var (
	// Commit the commit hash corresponding to the code that was built. Can be suffixed with `-dirty`
	Commit = "unknown"
	// BuildTime the time of build of the binary
	BuildTime = "unknown"
)

func NewMessage() string {
	return fmt.Sprintf("commit: '%s', build time: '%s'", Commit, BuildTime)
}
