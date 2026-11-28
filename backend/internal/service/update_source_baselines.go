package service

// These checkpoints record imported/reviewed source history, independently of
// this project's release version. A reviewed checkpoint does not imply every
// feature was merged. Update only after the corresponding review/sync completes.
const (
	forkImportedVersion    = "2.4.5"
	forkImportedCommit     = "1e242d7a64c02d93f4f0c68023a436b99c3e8c16"
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
