package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScheduledQualityDefaultsAndLegacyConsent(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		enabled  bool
		want     bool
	}{
		{"new_default", "", false, false},
		{"new_explicit_opt_in", "", true, true},
		{"disabled_invalid_config", QualityProviderChanshui, false, false},
		{"legacy_pelican", QualityProviderPelican, true, false},
		{"unsupported_legacy", "unknown-provider", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := &ScheduledTestPlan{ModelID: "gpt-test", QualityProvider: tc.provider, QualityCheckEnabled: tc.enabled}
			if tc.name == "disabled_invalid_config" {
				plan.QualityConfig.BaseURL = "http://localhost/old-config"
			}
			require.NoError(t, normalizeScheduledTestPlan(plan))
			require.Equal(t, QualityProviderChanshui, plan.QualityProvider)
			require.Equal(t, tc.want, plan.QualityCheckEnabled)
		})
	}
	for _, provider := range []string{"", QualityProviderPelican, "unknown-provider"} {
		plan := &ScheduledTestPlan{QualityProvider: provider, QualityCheckEnabled: true, ActiveAudit: &ChanshuiAuditView{ID: "legacy"}}
		NormalizeStoredScheduledQualityPlan(plan)
		require.Equal(t, QualityProviderChanshui, plan.QualityProvider)
		require.False(t, plan.QualityCheckEnabled)
		require.Nil(t, plan.ActiveAudit)
	}
}

func TestScheduledQualityDisabledAndLegacyPlansNeverCallAuditService(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		quality  bool
		enabled  bool
	}{
		{"quality_off", QualityProviderChanshui, false, true},
		{"legacy_pelican_on", QualityProviderPelican, true, true},
		{"legacy_empty_on", "", true, true},
		{"unknown_provider_on", "unknown-provider", true, true},
		{"plan_off", QualityProviderChanshui, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner, repo, _, plan, accounts := newChanshuiTestRunner(func(*http.Request) (*http.Response, error) {
				t.Fatal("disabled/legacy plans must never contact the audit service")
				return nil, nil
			})
			plan.QualityProvider, plan.QualityCheckEnabled, plan.Enabled = tc.provider, tc.quality, tc.enabled
			// Fail a plain upstream connectivity probe before any network access.
			accounts.readErr = errors.New("offline account fixture")
			runner.runOnePlan(context.Background(), plan)
			require.Zero(t, repo.reserves)
			require.Nil(t, repo.state)
			_, _, err := runner.runChanshuiPlan(context.Background(), plan)
			require.Error(t, err, "the audit entry point must also enforce consent")
			require.Zero(t, repo.reserves)
		})
	}
}

func TestChanshuiRechecksConsentBeforeEveryExternalRequest(t *testing.T) {
	for _, polling := range []bool{false, true} {
		for _, change := range []string{"quality_off", "plan_off", "provider_changed", "account_changed", "policy_changed", "revision_changed", "deleted", "read_error"} {
			t.Run(change+map[bool]string{false: "/submit", true: "/poll"}[polling], func(t *testing.T) {
				runner, repo, _, plan, _ := newChanshuiTestRunner(func(*http.Request) (*http.Response, error) {
					t.Fatal("stale consent must not cause an external request")
					return nil, nil
				})
				current := *plan
				repo.current = &current
				switch change {
				case "quality_off":
					current.QualityCheckEnabled = false
				case "plan_off":
					current.Enabled = false
				case "provider_changed":
					current.QualityProvider = QualityProviderPelican
				case "account_changed":
					current.AccountID++
				case "policy_changed":
					current.ModelID = "changed-model"
				case "revision_changed":
					current.UpdatedAt = time.Now()
				case "deleted":
					repo.current = nil
				case "read_error":
					repo.getErr = errors.New("offline database fixture")
				}
				if polling {
					repo.state = &ChanshuiAuditState{ID: "audit-1", PollURL: "/api/v1/audits/audit-1", ServiceURL: "https://audit.example", StartedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour).Unix()}
				}
				result, _, err := runner.runChanshuiPlan(context.Background(), plan)
				require.Error(t, err)
				require.Nil(t, result)
			})
		}
	}
}
