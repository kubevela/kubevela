package version

import "fmt"

const unspecified = "unspecified"

var (
	gitCommit = unspecified
	buildDate = unspecified
	version   = unspecified
)

// Info for git information
type Info struct {
	Version   string
	GitCommit string
	BuildDate string
}

// Get get info for git
func Get() Info {
	i := Info{
		Version:   version,
		GitCommit: gitCommit,
		BuildDate: buildDate,
	}
	if i.Version == "{version}" {
		i.Version = unspecified
	}
	if i.GitCommit == "{gitCommit}" {
		i.GitCommit = unspecified
	}
	if i.BuildDate == "{buildDate}" {
		i.BuildDate = unspecified
	}
	return i
}

func (i Info) String() string {
	return fmt.Sprintf(
		"Version: %s, GitCommit: %s, BuildDate: %s",
		i.Version,
		i.GitCommit,
		i.BuildDate,
	)
}
