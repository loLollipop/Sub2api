package service

// These checkpoints record imported/reviewed source history, independently of
// this project's release version. A reviewed checkpoint does not imply every
// feature was merged. Update only after the corresponding review/sync completes.
const (
	forkImportedVersion    = "2.1.16"
	forkImportedCommit     = "f7bde184592d6855d4b27d1f705a8aad13f49e3f"
	originalReviewedCommit = "1b2243a4f50621508e79aef4b498a2937a6238ee"
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
