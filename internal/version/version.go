package version

import (
	"fmt"
	"runtime/debug"
)

// Set via -ldflags "-X github.com/Slothtron/mwt/internal/version.Version=..."
var (
	Version   = "0.2.0"
	Commit    = "none"
	BuildDate = "unknown"
)

// Details holds resolved version metadata for the mwt binary.
type Details struct {
	Version   string
	Commit    string
	BuildDate string
}

// Info returns version details. Package Version is the baseline; Commit and
// BuildDate still fall back to runtime/debug.BuildInfo when left at defaults.
func Info() Details {
	d := Details{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return d
	}

	// Only adopt module version from BuildInfo when still at the historical
	// "dev" placeholder (ldflags / source baseline otherwise win).
	if d.Version == "dev" {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			d.Version = v
		}
	}

	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if d.Commit == "none" && s.Value != "" {
				d.Commit = shortCommit(s.Value)
			}
		case "vcs.time":
			if d.BuildDate == "unknown" && s.Value != "" {
				d.BuildDate = s.Value
			}
		}
	}

	return d
}

// String returns a single-line version report.
func String() string {
	d := Info()
	return fmt.Sprintf("mwt version %s (commit %s, built %s)", d.Version, d.Commit, d.BuildDate)
}

func shortCommit(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}
