package buildinfo

var (
	// GitCommit 表示构建时注入的提交哈希。
	GitCommit = ""
	// GitBranch 表示构建时注入的分支名。
	GitBranch = ""
	// GitTag 表示构建时注入的标签名。
	GitTag = ""
	// GitDirty 表示构建时工作区是否有未提交内容。
	GitDirty = ""
	// GitVersion 表示构建时注入的版本号。
	GitVersion = ""
	// BuildTime 表示构建时间。
	BuildTime = ""
	// BuildDir 表示构建目录。
	BuildDir = ""
)

// Version 返回当前构建版本，缺失构建信息时返回 dev。
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
