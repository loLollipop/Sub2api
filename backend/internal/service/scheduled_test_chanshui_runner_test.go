package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type chanshuiTestTransport func(*http.Request) (*http.Response, error)

func (f chanshuiTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func chanshuiTestResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

type chanshuiPlanRepoFake struct {
	scheduledTestPlanRepoStub
	current  *ScheduledTestPlan
	getErr   error
	state    *ChanshuiAuditState
	allow    bool
	reserves int
	delay    time.Duration
	deferred []time.Time
}

func (r *chanshuiPlanRepoFake) GetByID(context.Context, int64) (*ScheduledTestPlan, error) {
	return r.current, r.getErr
}

func (r *chanshuiPlanRepoFake) GetChanshuiAudit(context.Context, int64) (*ChanshuiAuditState, error) {
	if r.state == nil {
		return nil, nil
	}
	copy := *r.state
	return &copy, nil
}
func (r *chanshuiPlanRepoFake) SetChanshuiAudit(_ context.Context, _ int64, s *ChanshuiAuditState) error {
	r.state = s
	return nil
}
func (r *chanshuiPlanRepoFake) ReserveChanshuiCreate(context.Context) (bool, error) {
	r.reserves++
	ok := r.allow
	r.allow = false
	return ok, nil
}
func (r *chanshuiPlanRepoFake) DelayChanshuiCreate(_ context.Context, d time.Duration) error {
	r.delay = d
	return nil
}
func (r *chanshuiPlanRepoFake) DeferChanshuiPlan(_ context.Context, _ int64, next time.Time) error {
	r.deferred = append(r.deferred, next)
	return nil
}

func newChanshuiTestRunner(transport chanshuiTestTransport) (*ScheduledTestRunnerService, *chanshuiPlanRepoFake, *scheduledQualityResultRepo, *ScheduledTestPlan, *scheduledQualityAccountRepo) {
	account := &Account{ID: 2, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"api_key": "fixture-upstream-key", "base_url": "https://upstream.example/v1"}}
	results := &scheduledQualityResultRepo{}
	runner, accounts, _ := scheduledQualityRunner(account, results)
	runner.accountTestSvc.accountRepo = &chanshuiStateAccountFake{scheduledQualityAccountRepo: accounts}
	repo := &chanshuiPlanRepoFake{allow: true}
	runner.planRepo = repo
	runner.scheduledSvc = NewScheduledTestService(repo, results)
	runner.chanshuiClient = newCustomUsageClient()
	runner.chanshuiClient.Transport = transport
	plan := &ScheduledTestPlan{ID: 1, AccountID: 2, ModelID: "gpt-test", Enabled: true, MaxResults: 20, CronExpression: "*/5 * * * *", QualityCheckEnabled: true, QualityProvider: QualityProviderChanshui, QualityConfig: ChanshuiConfig{BaseURL: "https://audit.example", Protocol: "auto", Timeout: 360}}
	repo.current = plan
	return runner, repo, results, plan, accounts
}

func TestChanshuiCreateResumePollAndRedactedReport(t *testing.T) {
	posts, gets := 0, 0
	transport := chanshuiTestTransport(func(req *http.Request) (*http.Response, error) {
		require.Empty(t, req.Header.Get("Authorization"))
		require.Empty(t, req.Header.Get("Cookie"))
		require.Equal(t, "audit.example", req.URL.Host)
		if req.Method == http.MethodPost {
			posts++
			var body map[string]any
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			require.Equal(t, "https://upstream.example/v1", body["base_url"])
			require.Equal(t, "fixture-upstream-key", body["api_key"])
			require.Equal(t, "auto", body["protocol"])
			require.Equal(t, []any{"fingerprint"}, body["sections"])
			require.EqualValues(t, 360, body["timeout"])
			return chanshuiTestResponse(202, fmt.Sprintf(`{"id":"audit-1","status":"queued","poll_url":"/api/v1/audits/audit-1","expires_at":%d,"skipped":[{"id":"thinking","reason":"non-Claude"}]}`, time.Now().Add(24*time.Hour).Unix())), nil
		}
		gets++
		require.Equal(t, "/api/v1/audits/audit-1", req.URL.Path)
		if gets == 1 {
			return chanshuiTestResponse(200, `{"id":"audit-1","status":"running"}`), nil
		}
		return chanshuiTestResponse(200, `{"id":"audit-1","status":"done","probes":[{"id":"Q1","status":"queued"}],"verdict":{"fingerprint":{"top_model":"gpt-test"},"total":{"score":56,"max":100},"integrity":{"score":78},"tools":{"reason":"echo fixture-upstream-key","api_key":"must-not-persist"},"iq":{"score":null,"complete":false}}}`), nil
	})
	runner, repo, results, plan, accounts := newChanshuiTestRunner(transport)
	runner.runOnePlan(context.Background(), plan)
	require.Equal(t, 1, posts)
	require.Empty(t, results.results)
	require.NotNil(t, repo.state)
	require.Len(t, repo.deferred, 1)
	raw, err := json.Marshal(repo.state)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "fixture-upstream-key")
	// A replacement worker resumes the persisted GET, even after credential rotation.
	accounts.account.Credentials["api_key"] = "fixture-rotated-key"
	replacement := &ScheduledTestRunnerService{planRepo: runner.planRepo, scheduledSvc: runner.scheduledSvc, accountTestSvc: runner.accountTestSvc, chanshuiClient: runner.chanshuiClient, rateLimitSvc: runner.rateLimitSvc}
	replacement.runOnePlan(context.Background(), plan)
	require.Equal(t, "running", repo.state.Status)
	replacement.runOnePlan(context.Background(), plan)
	require.Equal(t, 1, posts)
	require.Equal(t, 2, gets)
	require.Equal(t, 1, repo.reserves)
	require.Nil(t, repo.state)
	require.Len(t, results.results, 1)
	require.Equal(t, "audit_pass", results.results[0].Status)
	require.Contains(t, results.results[0].ResponseText, `"score":56`)
	require.NotContains(t, results.results[0].ResponseText, "fixture-upstream-key")
	require.NotContains(t, results.results[0].ResponseText, "must-not-persist")
	require.Zero(t, accounts.pauses)
	require.Zero(t, accounts.recoveries)
	require.Zero(t, accounts.clears)
}

func TestChanshuiChangedConfigCannotApplyPendingDecision(t *testing.T) {
	runner, repo, results, plan, accounts := newChanshuiTestRunner(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		return chanshuiTestResponse(200, `{"id":"pending-fixture","status":"done","verdict":{"fingerprint":{"top_model":"different-model"}}}`), nil
	})
	repo.state = &ChanshuiAuditState{ID: "pending-fixture", PollURL: "https://audit.example/api/v1/audits/pending-fixture", ServiceURL: "https://audit.example", StartedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour).Unix(), DecisionConfigHash: chanshuiDecisionConfigHash(plan)}
	plan.ModelID = "changed-model"
	runner.runOnePlan(context.Background(), plan)
	require.Len(t, results.results, 1)
	require.Equal(t, "unknown", results.results[0].Status)
	require.True(t, accounts.account.Schedulable)
	require.Nil(t, repo.state)
	require.Zero(t, repo.reserves)
}

func TestChanshui429PersistsRetryAfterWithoutImmediateRetry(t *testing.T) {
	posts := 0
	runner, repo, results, plan, _ := newChanshuiTestRunner(func(req *http.Request) (*http.Response, error) {
		posts++
		r := chanshuiTestResponse(429, `{"error":"busy"}`)
		r.Header.Set("Retry-After", "137")
		return r, nil
	})
	runner.runOnePlan(context.Background(), plan)
	require.Equal(t, 137*time.Second, repo.delay)
	require.Len(t, repo.deferred, 1)
	require.True(t, repo.deferred[0].After(time.Now().Add(130*time.Second)))
	runner.runOnePlan(context.Background(), plan)
	require.Equal(t, 1, posts)
	require.Nil(t, repo.state)
	require.Empty(t, results.results)
}

func TestChanshuiSelectedSectionsAreSentAsAnArray(t *testing.T) {
	runner, repo, _, plan, _ := newChanshuiTestRunner(func(req *http.Request) (*http.Response, error) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
		require.Equal(t, []any{"tools", "cache"}, body["sections"])
		return chanshuiTestResponse(202, fmt.Sprintf(`{"id":"audit-selected","status":"queued","poll_url":"/api/v1/audits/audit-selected","expires_at":%d}`, time.Now().Add(time.Hour).Unix())), nil
	})
	plan.QualityConfig.Sections = []string{"tools", "cache"}
	plan.QualityConfig.StopCondition = &ChanshuiStopCondition{Match: "any", Rules: []ChanshuiStopRule{{Type: "section_status", Section: "tools", Status: "fail"}}}
	runner.runOnePlan(context.Background(), plan)
	require.NotNil(t, repo.state)
	require.Equal(t, "audit-selected", repo.state.ID)
}

func TestChanshuiInterruptedSubmissionIsNotReposted(t *testing.T) {
	runner, repo, results, plan, _ := newChanshuiTestRunner(func(*http.Request) (*http.Response, error) {
		t.Fatal("must not issue HTTP after an ambiguous POST")
		return nil, nil
	})
	repo.state = &ChanshuiAuditState{Status: "submitting", ServiceURL: "https://audit.example", StartedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour).Unix()}
	runner.runOnePlan(context.Background(), plan)
	require.Zero(t, repo.reserves)
	require.Nil(t, repo.state)
	require.Len(t, results.results, 1)
	require.Equal(t, "unknown", results.results[0].Status)
}

func TestChanshuiExpiredAndMissingJobsNeverRecreate(t *testing.T) {
	for _, code := range []int{404, 410} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			runner, repo, results, plan, _ := newChanshuiTestRunner(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodGet, req.Method)
				return chanshuiTestResponse(code, `{"error":"gone"}`), nil
			})
			repo.state = &ChanshuiAuditState{ID: "audit-1", PollURL: "/api/v1/audits/audit-1", ServiceURL: "https://audit.example", StartedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour).Unix()}
			runner.runOnePlan(context.Background(), plan)
			require.Zero(t, repo.reserves)
			require.Nil(t, repo.state)
			require.Len(t, results.results, 1)
			require.Equal(t, "unknown", results.results[0].Status)
		})
	}
}

func TestChanshuiCreationFailuresNeverLeakPayloadOrFollowRedirects(t *testing.T) {
	for _, code := range []int{302, 400} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			calls := 0
			runner, _, results, plan, _ := newChanshuiTestRunner(func(req *http.Request) (*http.Response, error) {
				calls++
				r := chanshuiTestResponse(code, `{"error":"fixture-upstream-key"}`)
				r.Header.Set("Location", "https://evil.example")
				return r, nil
			})
			runner.runOnePlan(context.Background(), plan)
			require.Equal(t, 1, calls)
			require.Len(t, results.results, 1)
			require.NotContains(t, results.results[0].ResponseText, "fixture-upstream-key")
			require.NotContains(t, results.results[0].ErrorMessage, "fixture-upstream-key")
		})
	}
}

type chanshuiStateAccountFake struct {
	*scheduledQualityAccountRepo
	changes int
}

func (r *chanshuiStateAccountFake) ApplyChanshuiQualityState(_ context.Context, _ int64, planID int64, _ time.Time, paused bool) (bool, error) {
	a := r.account
	if a == nil || a.Status != StatusActive {
		return false, nil
	}
	if a.Extra == nil {
		a.Extra = map[string]any{}
	}
	owners, _ := a.Extra[ChanshuiQualityPausesExtraKey].(map[string]any)
	if owners == nil {
		owners = map[string]any{}
	}
	key := fmt.Sprint(planID)
	if paused {
		if !a.Schedulable && len(owners) == 0 {
			return false, nil
		}
		if _, exists := owners[key]; exists {
			return false, nil
		}
		owners[key] = true
		a.Schedulable = false
		a.Extra[ChanshuiQualityPausesExtraKey] = owners
	} else {
		if a.Schedulable || owners[key] == nil {
			return false, nil
		}
		delete(owners, key)
		if len(owners) == 0 {
			delete(a.Extra, ChanshuiQualityPausesExtraKey)
			if !strings.HasPrefix(a.TempUnschedulableReason, scheduledQualityReasonPrefix) {
				a.Schedulable = true
			}
		} else {
			a.Extra[ChanshuiQualityPausesExtraKey] = owners
		}
	}
	r.changes++
	return true, nil
}

func TestChanshuiPauseUnknownAndRecoverOnlyOwnPlans(t *testing.T) {
	account := &Account{ID: 2, Status: StatusActive, Schedulable: true}
	results := &scheduledQualityResultRepo{}
	runner, base, _ := scheduledQualityRunner(account, results)
	accounts := &chanshuiStateAccountFake{scheduledQualityAccountRepo: base}
	runner.accountTestSvc.accountRepo = accounts
	p := &ScheduledTestPlan{ID: 1, AccountID: 2, MaxResults: 20, CronExpression: "*/5 * * * *", QualityCheckEnabled: true, QualityProvider: QualityProviderChanshui, AutoRecover: false}
	runner.completePlanRun(context.Background(), p, &ScheduledTestResult{Status: "audit_fail"})
	require.False(t, account.Schedulable, "first mismatch must stop, no two-strike delay")
	runner.completePlanRun(context.Background(), p, &ScheduledTestResult{Status: "unknown"})
	require.False(t, account.Schedulable)
	p2 := *p
	p2.ID = 3
	runner.completePlanRun(context.Background(), &p2, &ScheduledTestResult{Status: "audit_fail"})
	runner.completePlanRun(context.Background(), p, &ScheduledTestResult{Status: "audit_pass"})
	require.False(t, account.Schedulable, "another failing plan still owns a pause")
	runner.completePlanRun(context.Background(), &p2, &ScheduledTestResult{Status: "audit_pass"})
	require.True(t, account.Schedulable, "a confirmed pass recovers without legacy auto_recover")
	account.Schedulable = false // An unowned manual pause is not ours to clear.
	runner.completePlanRun(context.Background(), p, &ScheduledTestResult{Status: "audit_pass"})
	require.False(t, account.Schedulable)
}

func TestChanshuiPersistFailureCannotChangeScheduling(t *testing.T) {
	account := &Account{ID: 2, Status: StatusActive, Schedulable: true}
	results := &scheduledQualityResultRepo{createErr: fmt.Errorf("persist failed")}
	runner, base, _ := scheduledQualityRunner(account, results)
	accounts := &chanshuiStateAccountFake{scheduledQualityAccountRepo: base}
	runner.accountTestSvc.accountRepo = accounts
	runner.completePlanRun(context.Background(), &ScheduledTestPlan{ID: 1, AccountID: 2, MaxResults: 20, CronExpression: "*/5 * * * *", QualityCheckEnabled: true, QualityProvider: QualityProviderChanshui}, &ScheduledTestResult{Status: "audit_fail"})
	require.True(t, account.Schedulable)
	require.Zero(t, accounts.changes)
}

func TestChanshuiTerminalReportsUseOnlyValidEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, status, model, probeStatus, want string
		paused, changedConfig, wantSchedulable bool
	}{
		{name: "partial match recovers own pause", status: "partial", model: "gpt-test", probeStatus: "done", want: "audit_pass", paused: true, wantSchedulable: true},
		{name: "partial mismatch pauses", status: "partial", model: "different-model", probeStatus: "done", want: "audit_fail"},
		{name: "partial failed probe cannot recover", status: "partial", model: "gpt-test", probeStatus: "error", want: "unknown", paused: true},
		{name: "partial missing evidence cannot recover", status: "partial", probeStatus: "done", want: "unknown", paused: true},
		{name: "partial changed policy cannot recover", status: "partial", model: "gpt-test", probeStatus: "done", want: "unknown", paused: true, changedConfig: true},
		{name: "error cannot recover", status: "error", model: "gpt-test", probeStatus: "done", want: "unknown", paused: true},
		{name: "failed cannot recover", status: "failed", model: "gpt-test", probeStatus: "done", want: "unknown", paused: true},
		{name: "error cannot pause", status: "error", model: "different-model", probeStatus: "done", want: "unknown", wantSchedulable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gets := 0
			runner, repo, results, plan, accounts := newChanshuiTestRunner(func(req *http.Request) (*http.Response, error) {
				require.Equal(t, http.MethodGet, req.Method, "terminal audits must not trigger a new submission")
				gets++
				body := fmt.Sprintf(`{"id":"terminal-fixture","status":%q,"probes":[{"id":"fingerprint","status":%q},{"id":"iq","status":"error","error":"fixture-upstream-key"}],"verdict":{"fingerprint":{"top_model":%q}}}`, tc.status, tc.probeStatus, tc.model)
				return chanshuiTestResponse(200, body), nil
			})
			if tc.paused {
				accounts.account.Schedulable = false
				accounts.account.Extra = map[string]any{ChanshuiQualityPausesExtraKey: map[string]any{"1": true}}
			}
			repo.state = &ChanshuiAuditState{ID: "terminal-fixture", PollURL: "https://audit.example/api/v1/audits/terminal-fixture", ServiceURL: "https://audit.example", StartedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour).Unix(), DecisionConfigHash: chanshuiDecisionConfigHash(plan)}
			if tc.changedConfig {
				plan.ModelID = "changed-model"
			}
			runner.runOnePlan(context.Background(), plan)
			require.Equal(t, 1, gets)
			require.Zero(t, repo.reserves)
			require.Empty(t, repo.deferred, "terminal reports must finish this run")
			require.Nil(t, repo.state)
			require.Len(t, results.results, 1)
			result := results.results[0]
			require.Equal(t, tc.want, result.Status)
			require.Equal(t, tc.wantSchedulable, accounts.account.Schedulable)
			var report map[string]any
			require.NoError(t, json.Unmarshal([]byte(result.ResponseText), &report))
			require.Equal(t, tc.status, report["audit_status"])
			require.Contains(t, report, "probes")
			require.NotContains(t, result.ResponseText, "fixture-upstream-key")
			require.NotContains(t, result.ErrorMessage, "无法识别")
		})
	}
}

func TestChanshuiTerminalCreateResponsesAreRetainedAndPolled(t *testing.T) {
	for _, status := range []string{"done", "partial", "error", "failed"} {
		t.Run(status, func(t *testing.T) {
			posts, gets := 0, 0
			runner, repo, results, plan, _ := newChanshuiTestRunner(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodPost {
					posts++
					return chanshuiTestResponse(202, fmt.Sprintf(`{"id":"created-fixture","status":%q,"poll_url":"/api/v1/audits/created-fixture","expires_at":%d}`, status, time.Now().Add(time.Hour).Unix())), nil
				}
				require.Equal(t, http.MethodGet, req.Method)
				gets++
				return chanshuiTestResponse(200, fmt.Sprintf(`{"id":"created-fixture","status":%q,"verdict":{"fingerprint":{"top_model":"gpt-test"}}}`, status)), nil
			})
			runner.runOnePlan(context.Background(), plan)
			require.Empty(t, results.results)
			require.NotNil(t, repo.state)
			require.Equal(t, status, repo.state.Status)
			require.Equal(t, "created-fixture", repo.state.ID)
			runner.runOnePlan(context.Background(), plan)
			require.Equal(t, 1, posts)
			require.Equal(t, 1, gets)
			require.Equal(t, 1, repo.reserves)
			require.Nil(t, repo.state)
			require.Len(t, results.results, 1)
			if status == "error" || status == "failed" {
				require.Equal(t, "unknown", results.results[0].Status)
			} else {
				require.Equal(t, "audit_pass", results.results[0].Status)
			}
		})
	}
}
