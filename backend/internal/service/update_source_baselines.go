package service

// These checkpoints record imported/reviewed source history, independently of
// this project's release version. A reviewed checkpoint does not imply every
// feature was merged. Update only after the corresponding review/sync completes.
const (
	forkImportedVersion    = "2.2.11"
	forkImportedCommit     = "6c6a9efc96573c879b049265bc57a2d42509bda1"
	originalReviewedCommit = "b0a02a3304a0a78cdd003ddda1dd23ba02c03084"
)

type updateSourceBaseline struct {
	repository string
	branch     string
	commit     string
	// Release checkpoints are separate from reviewed main history above.
	releaseTag    string
	releaseCommit string
}

func defaultUpdateSourceBaselines() []updateSourceBaseline {
	return []updateSourceBaseline{
		{repository: "kiss-kedaya/sub2api", branch: "main", commit: forkImportedCommit, releaseTag: "v2.2.11", releaseCommit: forkImportedCommit},
		{repository: "Wei-Shaw/sub2api", branch: "main", commit: originalReviewedCommit, releaseTag: "v0.2.17", releaseCommit: "060d0160417782cd962a0c7f460f6cd4565d4916"},
	}
}
