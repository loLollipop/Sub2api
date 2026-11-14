package service

// These checkpoints record imported/reviewed source history, independently of
// this project's release version. A reviewed checkpoint does not imply every
// feature was merged. Update only after the corresponding review/sync completes.
const (
	forkImportedVersion    = "2.3.76"
	forkImportedCommit     = "f3956f1a7f31254e47eaf8cbb6cd9d83f2998b1d"
	originalReviewedCommit = "10ba8f9b4dc2e6023d4a9879ab77b21841babd10"
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
		{repository: "Wei-Shaw/sub2api", branch: "main", commit: originalReviewedCommit, releaseTag: "v0.2.21", releaseCommit: "10ba8f9b4dc2e6023d4a9879ab77b21841babd10"},
	}
}
