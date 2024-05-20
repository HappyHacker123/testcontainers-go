package release

import (
	"testing"
)

func TestNewReleaseManager(t *testing.T) {
	testCases := []struct {
		name     string
		branch   string
		bumpType string
		dryRun   bool
	}{
		{
			name:     "main branch, minor bump, dry run",
			branch:   "main",
			bumpType: "minor",
			dryRun:   true,
		},
		{
			name:     "main branch, minor bump, no dry run",
			branch:   "main",
			bumpType: "minor",
			dryRun:   false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(tt *testing.T) {
			tt.Parallel()

			r := NewReleaseManager(tc.branch, tc.bumpType, tc.dryRun)

			if r == nil {
				tt.Error("expected a non-nil Releaser")
			}

			if tc.dryRun {
				if _, ok := r.(*dryRunReleaseManager); !ok {
					tt.Error("expected a *dryRunReleaseManager")
				}
			} else {
				if _, ok := r.(*releaseManager); !ok {
					tt.Error("expected a *releaseManager")
				}
			}
		})
	}
}
