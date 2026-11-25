package service

// These checkpoints record imported/reviewed source history, independently of
// this project's release version. A reviewed checkpoint does not imply every
// feature was merged. Update only after the corresponding review/sync completes.
const (
	forkImportedVersion    = "2.3.164"
	forkImportedCommit     = "078bc0375fa0d842c6c0b6872216b997e3427b08"
	originalReviewedCommit = "348997267d756eab5729bb6b78a275a15d18796a"
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
		{repository: "kiss-kedaya/sub2api", branch: "main", commit: forkImportedCommit, releaseTag: "v" + forkImportedVersion, releaseCommit: forkImportedCommit},
		{repository: "Wei-Shaw/sub2api", branch: "main", commit: originalReviewedCommit, releaseTag: "v0.2.25", releaseCommit: "348997267d756eab5729bb6b78a275a15d18796a"},
	}
}
