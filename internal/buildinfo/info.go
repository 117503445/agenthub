package buildinfo

var (
	GitCommit  = ""
	GitBranch  = ""
	GitTag     = ""
	GitDirty   = ""
	GitVersion = ""
	BuildTime  = ""
	BuildDir   = ""
)

func Version() string {
	if GitVersion != "" {
		return GitVersion
	}
	if GitCommit != "" {
		if len(GitCommit) > 12 {
			return GitCommit[:12]
		}
		return GitCommit
	}
	return "dev"
}
