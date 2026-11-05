//go:build integration

package repository

import (
	"context"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// 这一组用例覆盖用户行上的 lost update：调用方手里的快照可能早于并发发生的
// 原子写入（扣费、状态变更、限额调整、分组授予）。Update 只写显式声明的列，
// 未声明的列一律保持库中当前值，因此陈旧快照不会回滚这些并发结果。

func (s *UserRepoSuite) TestUpdate_DoesNotRevertConcurrentBalanceDeduction() {
	user := s.mustCreateUser(&service.User{
		Email:    "lost-update-balance@example.com",
		Username: "before",
		Balance:  0.30,
	})

	// 调用方在扣费之前读到的旧快照（余额还是 0.30）。
	stale, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().InDelta(0.30, stale.Balance, 1e-9)

	// 与之并发：计费按原子方式扣费。
	s.Require().NoError(s.repo.DeductBalance(s.ctx, user.ID, 0.25), "DeductBalance")

	// 基于旧快照的资料更新这时才落库。
	stale.Username = "after"
	s.Require().NoError(
		s.repo.Update(s.ctx, stale, service.UserUpdateFields{Username: true}),
		"Update",
	)

	got, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("after", got.Username, "declared column must still be written")
	s.Require().InDelta(0.05, got.Balance, 1e-9, "balance must not be reverted by a stale profile save")
}

// 同理，风控自动封禁把 status 置为 disabled 后，
// 基于旧快照的资料更新不得把 status 刷回 active。
func (s *UserRepoSuite) TestUpdate_DoesNotRevertConcurrentBan() {
	user := s.mustCreateUser(&service.User{
		Email:    "lost-update-ban@example.com",
		Username: "before",
		Status:   service.StatusActive,
	})

	stale, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Equal(service.StatusActive, stale.Status)

	banned, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID for ban")
	banned.Status = service.StatusDisabled
	s.Require().NoError(
		s.repo.Update(s.ctx, banned, service.UserUpdateFields{Status: true}),
		"ban",
	)

	stale.Username = "after"
	s.Require().NoError(
		s.repo.Update(s.ctx, stale, service.UserUpdateFields{Username: true}),
		"stale profile save",
	)

	got, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("after", got.Username)
	s.Require().Equal(service.StatusDisabled, got.Status, "ban must survive a stale profile save")
}

// 未声明的列不写，也意味着并发的限额调整不会被资料保存回滚。
func (s *UserRepoSuite) TestUpdate_DoesNotRevertConcurrentLimitChanges() {
	user := s.mustCreateUser(&service.User{
		Email:       "lost-update-limits@example.com",
		Username:    "before",
		Concurrency: 3,
		RPMLimit:    30,
	})

	stale, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID")

	concurrency, rpmLimit := 9, 90
	affected, err := s.repo.BatchUpdateLimits(s.ctx, []int64{user.ID}, &concurrency, &rpmLimit)
	s.Require().NoError(err, "BatchUpdateLimits")
	s.Require().Equal(1, affected)

	stale.Username = "after"
	s.Require().NoError(
		s.repo.Update(s.ctx, stale, service.UserUpdateFields{Username: true}),
		"stale profile save",
	)

	got, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal(9, got.Concurrency, "concurrency must not be reverted")
	s.Require().Equal(90, got.RPMLimit, "rpm limit must not be reverted")
}

// AllowedGroups 只在显式声明时才同步，否则并发授予的分组权限会被旧快照删掉。
func (s *UserRepoSuite) TestUpdate_DoesNotRevertConcurrentAllowedGroupGrant() {
	group := s.mustCreateGroup("lost-update-group")
	user := s.mustCreateUser(&service.User{
		Email:    "lost-update-groups@example.com",
		Username: "before",
	})

	stale, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Empty(stale.AllowedGroups)

	s.Require().NoError(s.repo.AddGroupToAllowedGroups(s.ctx, user.ID, group.ID), "AddGroupToAllowedGroups")

	stale.Username = "after"
	s.Require().NoError(
		s.repo.Update(s.ctx, stale, service.UserUpdateFields{Username: true}),
		"stale profile save",
	)

	got, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal([]int64{group.ID}, got.AllowedGroups, "granted group must not be reverted")
}

func (s *UserRepoSuite) TestAdjustBalance_AppliesDeltaAndReportsChange() {
	user := s.mustCreateUser(&service.User{Email: "adjust-balance@example.com", Balance: 10})

	change, err := s.repo.AdjustBalance(s.ctx, user.ID, 5)
	s.Require().NoError(err, "AdjustBalance add")
	s.Require().InDelta(10, change.Old, 1e-9)
	s.Require().InDelta(15, change.New, 1e-9)

	change, err = s.repo.AdjustBalance(s.ctx, user.ID, -5)
	s.Require().NoError(err, "AdjustBalance subtract")
	s.Require().InDelta(15, change.Old, 1e-9)
	s.Require().InDelta(10, change.New, 1e-9)

	got, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().InDelta(10, got.Balance, 1e-9)
}

func (s *UserRepoSuite) TestAdjustBalance_RefusesNegativeResult() {
	user := s.mustCreateUser(&service.User{Email: "adjust-balance-negative@example.com", Balance: 3})

	change, err := s.repo.AdjustBalance(s.ctx, user.ID, -4)
	s.Require().ErrorIs(err, service.ErrBalanceNegative)
	s.Require().InDelta(3, change.Old, 1e-9, "error must report the real current balance")
	s.Require().InDelta(-1, change.New, 1e-9)

	got, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().InDelta(3, got.Balance, 1e-9, "refused adjustment must not write")
}

func (s *UserRepoSuite) TestAdjustBalance_UserNotFound() {
	_, err := s.repo.AdjustBalance(s.ctx, 99999999, 1)
	s.Require().ErrorIs(err, service.ErrUserNotFound)
}

func (s *UserRepoSuite) TestSetBalance_ReplacesValueAndReportsPrevious() {
	user := s.mustCreateUser(&service.User{Email: "set-balance@example.com", Balance: 7})

	change, err := s.repo.SetBalance(s.ctx, user.ID, 2)
	s.Require().NoError(err, "SetBalance")
	s.Require().InDelta(7, change.Old, 1e-9)
	s.Require().InDelta(2, change.New, 1e-9)

	got, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().InDelta(2, got.Balance, 1e-9)
}

func (s *UserRepoSuite) TestSetBalance_ReportsBalanceAfterConcurrentDeduction() {
	for _, value := range []float64{20, 10} {
		for _, commit := range []bool{true, false} {
			s.Run(fmt.Sprintf("value_%g_commit_%t", value, commit), func() {
				user := s.mustCreateUser(&service.User{
					Email:   fmt.Sprintf("set-balance-concurrent-%g-%t@example.com", value, commit),
					Balance: 10,
				})
				ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
				defer cancel()

				deductTx, err := s.client.Tx(ctx)
				s.Require().NoError(err, "begin deduction transaction")
				defer func() { _ = deductTx.Rollback() }()
				deductCtx := dbent.NewTxContext(ctx, deductTx)
				s.Require().NoError(s.repo.DeductBalance(deductCtx, user.ID, 3))

				setTx, err := s.client.Tx(ctx)
				s.Require().NoError(err, "begin set transaction")
				// Release the blocker before rolling back a possibly waiting SetBalance.
				defer func() {
					cancel()
					_ = deductTx.Rollback()
					_ = setTx.Rollback()
				}()
				setCtx := dbent.NewTxContext(ctx, setTx)
				pidRows, err := setTx.Client().QueryContext(ctx, "SELECT pg_backend_pid()")
				s.Require().NoError(err)
				s.Require().True(pidRows.Next())
				var setPID int
				s.Require().NoError(pidRows.Scan(&setPID))
				s.Require().NoError(pidRows.Close())

				type result struct {
					change service.BalanceChange
					err    error
				}
				results := make(chan result, 1)
				go func() {
					change, err := s.repo.SetBalance(setCtx, user.ID, value)
					results <- result{change: change, err: err}
				}()

				// Observe the actual PostgreSQL lock wait before releasing the deduction.
				// A fixed delay could let SetBalance start only after the commit.
				waitCtx, stopWaiting := context.WithTimeout(ctx, 5*time.Second)
				defer stopWaiting()
				poll := time.NewTicker(10 * time.Millisecond)
				defer poll.Stop()
				for {
					var waiting bool
					err := integrationDB.QueryRowContext(waitCtx, `
						SELECT EXISTS (
							SELECT 1 FROM pg_stat_activity
							WHERE pid = $1 AND wait_event_type = 'Lock'
							AND cardinality(pg_blocking_pids(pid)) > 0
						)`, setPID).Scan(&waiting)
					s.Require().NoError(err, "observe SetBalance lock wait")
					if waiting {
						break
					}
					select {
					case got := <-results:
						s.Require().FailNow("SetBalance returned before the deduction released its lock", "%+v", got)
					case <-waitCtx.Done():
						s.Require().NoError(waitCtx.Err(), "SetBalance must wait for the deduction lock")
					case <-poll.C:
					}
				}
				s.Require().NoError(deductTx.Commit(), "commit concurrent deduction")

				var got result
				select {
				case got = <-results:
				case <-ctx.Done():
					s.Require().NoError(ctx.Err(), "SetBalance must finish after the lock is released")
				}
				s.Require().NoError(got.err, "SetBalance")
				s.Require().InDelta(7, got.change.Old, 1e-9, "Old must include the committed deduction")
				s.Require().InDelta(value, got.change.New, 1e-9)
				s.Require().InDelta(value-7, got.change.New-got.change.Old, 1e-9, "source record and cache delta")

				persisted, err := s.repo.GetByID(ctx, user.ID)
				s.Require().NoError(err)
				s.Require().InDelta(7, persisted.Balance, 1e-9, "SetBalance must stay in the context transaction")
				want := float64(7)
				if commit {
					s.Require().NoError(setTx.Commit())
					want = value
				} else {
					s.Require().NoError(setTx.Rollback())
				}
				persisted, err = s.repo.GetByID(ctx, user.ID)
				s.Require().NoError(err)
				s.Require().InDelta(want, persisted.Balance, 1e-9, "transaction outcome must control the balance")
			})
		}
	}
}

func (s *UserRepoSuite) TestSetBalance_RejectsNegativeValue() {
	user := s.mustCreateUser(&service.User{Email: "set-balance-negative@example.com", Balance: 7})

	_, err := s.repo.SetBalance(s.ctx, user.ID, -1)
	s.Require().ErrorIs(err, service.ErrBalanceNegative)

	got, err := s.repo.GetByID(s.ctx, user.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().InDelta(7, got.Balance, 1e-9)
}

func (s *UserRepoSuite) TestSetBalance_UserNotFound() {
	_, err := s.repo.SetBalance(s.ctx, 99999999, 1)
	s.Require().ErrorIs(err, service.ErrUserNotFound)
}
