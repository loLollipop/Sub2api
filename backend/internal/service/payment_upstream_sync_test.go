//go:build unit

package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type captureRefundQueryTimeProvider struct {
	refundProviderTestDouble
	calls   int
	request payment.RefundQueryRequest
}

func (p *captureRefundQueryTimeProvider) QueryRefund(_ context.Context, req payment.RefundQueryRequest) (*payment.RefundResponse, error) {
	p.calls++
	p.request = req
	return &payment.RefundResponse{RefundID: req.RefundID, Status: payment.ProviderStatusPending}, nil
}

func TestQueryAndFinalizeRefundForwardsLatestAuditTime(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "query-audit-time")
	requestedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	_, err := client.PaymentAuditLog.Delete().Where(paymentauditlog.ActionEQ("REFUND_PENDING")).Exec(ctx)
	require.NoError(t, err)
	for _, id := range []string{"older", "latest"} {
		_, err = client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(order.ID, 10)).SetAction("REFUND_PENDING").SetOperator("admin").SetCreatedAt(requestedAt).SetDetail(`{"refundID":"` + id + `"}`).Save(ctx)
		require.NoError(t, err)
	}
	_, err = client.PaymentAuditLog.Create().SetOrderID(strconv.FormatInt(order.ID, 10)).SetAction("REFUND_PENDING").SetOperator("admin").SetCreatedAt(requestedAt.Add(-time.Hour)).SetDetail(`{"refundID":"older-time-newer-id"}`).Save(ctx)
	require.NoError(t, err)
	provider := &captureRefundQueryTimeProvider{}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}
	_, err = svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, 1, provider.calls)
	require.Equal(t, "latest", provider.request.RefundID)
	require.True(t, requestedAt.Equal(provider.request.RequestedAt))
}

func TestQueryAndFinalizeRefundUnreadableAuditFailsBeforeProvider(t *testing.T) {
	for _, mode := range []string{"malformed JSON", "null JSON", "database failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createPendingRefundOrderForTest(t, ctx, client, "audit-read-failure")
			if mode != "database failure" {
				detail := `{"refundID":`
				if mode == "null JSON" {
					detail = "null"
				}
				_, err := client.PaymentAuditLog.Update().Where(paymentauditlog.ActionEQ("REFUND_PENDING")).SetDetail(detail).Save(ctx)
				require.NoError(t, err)
			} else {
				client.PaymentAuditLog.Intercept(dbent.InterceptFunc(func(dbent.Querier) dbent.Querier {
					return dbent.QuerierFunc(func(context.Context, dbent.Query) (dbent.Value, error) {
						return nil, errors.New("injected audit read failure")
					})
				}))
			}
			provider := &captureRefundQueryTimeProvider{}
			restore := replacePaymentProviderFactoryForTest(t, provider)
			defer restore()
			svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}
			result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
			require.Error(t, err)
			require.Nil(t, result)
			require.Zero(t, provider.calls)
			current, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusRefundPending, current.Status)
		})
	}
}

func TestQueryAndFinalizeRefundMissingLegacyAuditKeepsUnknownTime(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPendingRefundOrderForTest(t, ctx, client, "legacy-audit-missing")
	_, err := client.PaymentAuditLog.Delete().Exec(ctx)
	require.NoError(t, err)
	provider := &captureRefundQueryTimeProvider{}
	restore := replacePaymentProviderFactoryForTest(t, provider)
	defer restore()
	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}
	_, err = svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, 1, provider.calls)
	require.True(t, provider.request.RequestedAt.IsZero())
}

func TestFulfillmentRejectsWrongTypeBeforeBalanceSideEffects(t *testing.T) {
	for _, orderType := range []string{payment.OrderTypeSubscription, "unsupported"} {
		t.Run(orderType, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusPaid, time.Now())
			_, err := client.PaymentOrder.UpdateOneID(order.ID).SetOrderType(orderType).Save(ctx)
			require.NoError(t, err)
			svc := &PaymentService{entClient: client}
			require.Equal(t, "INVALID_ORDER_TYPE", infraerrors.Reason(svc.ExecuteBalanceFulfillment(ctx, order.ID)))
			current, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, OrderStatusPaid, current.Status)
		})
	}
}

func TestFulfillmentUnsupportedTypeClosesOnlyPaidOrRecharging(t *testing.T) {
	for _, status := range []string{OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted, OrderStatusRefundPending, OrderStatusFailed} {
		t.Run(status, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, status, time.Now())
			_, err := client.PaymentOrder.UpdateOneID(order.ID).SetOrderType("unsupported").Save(ctx)
			require.NoError(t, err)
			svc := &PaymentService{entClient: client}
			require.NoError(t, svc.executeFulfillment(ctx, order.ID))
			current, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			wantStatus, wantAudits := status, 0
			if status == OrderStatusPaid || status == OrderStatusRecharging {
				wantStatus, wantAudits = OrderStatusFailed, 1
			}
			require.Equal(t, wantStatus, current.Status)
			count, err := client.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("FULFILLMENT_FAILED")).Count(ctx)
			require.NoError(t, err)
			require.Equal(t, wantAudits, count)
		})
	}
}

func TestFulfillmentUnsupportedTypeMutationFailureIsReturned(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusPaid, time.Now())
	_, err := client.PaymentOrder.UpdateOneID(order.ID).SetOrderType("unsupported").Save(ctx)
	require.NoError(t, err)
	client.PaymentOrder.Use(func(dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(context.Context, dbent.Mutation) (dbent.Value, error) {
			return nil, errors.New("injected order update failure")
		})
	})
	svc := &PaymentService{entClient: client}
	require.ErrorContains(t, svc.executeFulfillment(ctx, order.ID), "injected order update failure")
	current, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPaid, current.Status)
}

func TestRetryFulfillmentRejectsUnsupportedTypeBeforeRetryAudit(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusFailed, time.Now())
	_, err := client.PaymentOrder.UpdateOneID(order.ID).SetOrderType("unsupported").SetPaidAt(time.Now()).Save(ctx)
	require.NoError(t, err)
	svc := &PaymentService{entClient: client}
	require.Equal(t, "INVALID_ORDER_TYPE", infraerrors.Reason(svc.RetryFulfillment(ctx, order.ID)))
	count, err := client.PaymentAuditLog.Query().Where(paymentauditlog.ActionEQ("RECHARGE_RETRY")).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}
