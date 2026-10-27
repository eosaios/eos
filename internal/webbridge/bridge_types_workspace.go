package webbridge

// Workspace 域 DTO：工作区卡片、远程仓库状态、worktree、迁移边界。
// JSON 字段语义与前端契约一致，仅做类型归属拆分。

type WorkspaceCard struct {
	Path             string `json:"path"`
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	Trusted          bool   `json:"trusted"`
	Active           bool   `json:"active"`
	Removable        bool   `json:"removable"`
	SessionCount     int    `json:"sessionCount"`
	CurrentSessionID string `json:"currentSessionId,omitempty"`
	Platform         string `json:"platform,omitempty"`
	RepoURL          string `json:"repoUrl,omitempty"`
	Owner            string `json:"owner,omitempty"`
	Repo             string `json:"repo,omitempty"`
	Branch           string `json:"branch,omitempty"`
	DefaultBranch    string `json:"defaultBranch,omitempty"`
	Account          string `json:"account,omitempty"`
	Exists           bool   `json:"exists,omitempty"`
	ID               string `json:"id,omitempty"`
}

type RemoteRepoState struct {
	Mode          string `json:"mode"`
	Platform      string `json:"platform"`
	RepoURL       string `json:"repoUrl"`
	Owner         string `json:"owner"`
	Repo          string `json:"repo"`
	DefaultBranch string `json:"defaultBranch"`
	WorkingBranch string `json:"workingBranch"`
	LocalPath     string `json:"localPath"`
	AccountLogin  string `json:"accountLogin"`
	AccountName   string `json:"accountName"`
	UpdatedAt     string `json:"updatedAt"`
}

type RemoteRepoFlowRequest struct {
	RepoURL  string `json:"repoUrl"`
	Platform string `json:"platform,omitempty"`
	Branch   string `json:"branch,omitempty"`
	Goal     string `json:"goal,omitempty"`
}

type WorktreeCard struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Branch    string `json:"branch"`
	Head      string `json:"head"`
	Active    bool   `json:"active"`
	Removable bool   `json:"removable"`
}

type MigrationBoundary struct {
	Name    string   `json:"name"`
	Scope   string   `json:"scope"`
	Targets []string `json:"targets"`
	Notes   []string `json:"notes"`
}

type DirectoryEntry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
}

type DirectoryListing struct {
	Path    string           `json:"path"`
	Entries []DirectoryEntry `json:"entries"`
}
