package types

type FileInfo struct {
	Path         string
	RelativePath string
	Size         int64
	GitChanges   int
	Content      []byte
	Language     string
}

type RepoAnalyzer struct {
	RepoURL           string
	LocalPath         string
	OutputFile        string
	Files             []FileInfo
	GitignorePatterns []string
}

type IssueProcessing struct {
	BranchName   string
	IssueTitle   string
	RepoName     string
	AnalysisFile string
}
