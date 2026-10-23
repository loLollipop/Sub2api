package handler

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

// unblockedGrokHandlerBillingRepo keeps unrelated handler tests on their intended
// paths while satisfying the durable refusal check. Billing is never expected.
type unblockedGrokHandlerBillingRepo struct {
	service.UsageBillingRepository
	t testing.TB
}

var _ service.UsageBillingRepository = (*unblockedGrokHandlerBillingRepo)(nil)
var _ service.MediaBillingJobRepository = (*unblockedGrokHandlerBillingRepo)(nil)
var _ service.GrokRefusalRepository = (*unblockedGrokHandlerBillingRepo)(nil)

func (r *unblockedGrokHandlerBillingRepo) unexpected() error {
	r.t.Helper()
	r.t.Error("unexpected durable billing operation")
	return errors.New("unexpected durable billing operation")
}

func (*unblockedGrokHandlerBillingRepo) FindGrokRefusal(context.Context, int64, []string) (bool, error) {
	return false, nil
}

func (r *unblockedGrokHandlerBillingRepo) CommitGrokRefusal(context.Context, *service.MediaBillingJob, []string, []string) (*service.MediaBillingJob, bool, error) {
	return nil, false, r.unexpected()
}

func (r *unblockedGrokHandlerBillingRepo) Apply(context.Context, *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	return nil, r.unexpected()
}

func (r *unblockedGrokHandlerBillingRepo) CreateMediaBillingJob(context.Context, *service.MediaBillingJob) (*service.MediaBillingJob, error) {
	return nil, r.unexpected()
}

func (r *unblockedGrokHandlerBillingRepo) SubmitMediaBillingJob(context.Context, string, string, json.RawMessage) error {
	return r.unexpected()
}

func (r *unblockedGrokHandlerBillingRepo) GetMediaBillingJob(context.Context, int64, int64, string) (*service.MediaBillingJob, error) {
	return nil, r.unexpected()
}

func (r *unblockedGrokHandlerBillingRepo) GetMediaBillingJobByID(context.Context, string) (*service.MediaBillingJob, error) {
	return nil, r.unexpected()
}

func (r *unblockedGrokHandlerBillingRepo) PrepareMediaBillingIntent(context.Context, string, *service.MediaBillingIntent) (*service.MediaBillingIntent, error) {
	return nil, r.unexpected()
}

func (r *unblockedGrokHandlerBillingRepo) ApplyMediaBillingJob(context.Context, string) (*service.UsageBillingApplyResult, error) {
	return nil, r.unexpected()
}

func (r *unblockedGrokHandlerBillingRepo) FailMediaBillingJob(context.Context, string) (*service.UsageBillingApplyResult, error) {
	return nil, r.unexpected()
}

func (r *unblockedGrokHandlerBillingRepo) CompleteMediaBillingJob(context.Context, string) error {
	return r.unexpected()
}

func (*unblockedGrokHandlerBillingRepo) ListMediaBillingJobs(context.Context, int) ([]*service.MediaBillingJob, error) {
	return nil, nil
}

func (r *unblockedGrokHandlerBillingRepo) RetryMediaBillingJob(context.Context, string, time.Duration) error {
	return r.unexpected()
}

func (r *unblockedGrokHandlerBillingRepo) ReconcileMediaBillingJob(_ context.Context, id string) error {
	return r.unexpected()
}
