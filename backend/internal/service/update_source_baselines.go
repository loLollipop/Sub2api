package service

// These checkpoints record imported/reviewed source history, independently of
// this project's release version. A reviewed checkpoint does not imply every
// feature was merged. Update only after the corresponding review/sync completes.
const (
	forkImportedVersion    = "2.2.2"
	forkImportedCommit     = "648c17cc14846c901726f866fa76059d61aacc75"
	originalReviewedCommit = "b0a02a3304a0a78cdd003ddda1dd23ba02c03084"
)

type updateSourceBaseline struct {
	repository string
	branch     string
	commit     string
}

func defaultUpdateSourceBaselines() []updateSourceBaseline {
	return []updateSourceBaseline{
		{repository: "kiss-kedaya/sub2api", branch: "main", commit: forkImportedCommit},
		{repository: "Wei-Shaw/sub2api", branch: "main", commit: originalReviewedCommit},
	}
}
