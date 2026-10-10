package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
)

func TestFilterUsageInflightKeepsLiveRowAndDropsFinished(t *testing.T) {
	now := time.Date(2026, 10, 9, 3, 0, 0, 0, time.UTC)
	rows := []UsageInflightSnapshot{
		{RequestID: "live", UserID: 1, APIKeyID: 7, Model: "deepseek-v4", StartedAt: now.Add(-2 * time.Second), ReservedAmount: 0.2},
		{RequestID: "done", UserID: 1, Model: "deepseek-v4", StartedAt: now},
		{RequestID: "other", UserID: 1, Model: "gpt", StartedAt: now},
	}
	got := filterUsageInflight(rows, usagestats.UsageLogFilters{Model: "deepseek-v4"}, map[string]struct{}{"done": {}})
	if len(got) != 1 || got[0].RequestID != "live" || got[0].ReservedAmount != 0.2 {
		t.Fatalf("got %+v", got)
	}
}

func TestQueryIncludesUsageInflightDefaultsOn(t *testing.T) {
	if !QueryIncludesUsageInflight("") || !QueryIncludesUsageInflight("yes") {
		t.Fatal("empty or invalid should include inflight")
	}
	if QueryIncludesUsageInflight("false") {
		t.Fatal("false should exclude inflight")
	}
}

func TestPageUsageInflightReturnsTen(t *testing.T) {
	rows := make([]UsageInflightSnapshot, 0, 12)
	for i := 0; i < 12; i++ {
		rows = append(rows, UsageInflightSnapshot{RequestID: strconv.Itoa(i)})
	}
	page, total := PageUsageInflight(rows, 1, 100)
	if total != 12 || len(page) != 10 || page[0].RequestID != "0" {
		t.Fatalf("first page %+v total %d", page, total)
	}
	page, total = PageUsageInflight(rows, 2, 10)
	if total != 12 || len(page) != 2 || page[0].RequestID != "10" {
		t.Fatalf("second page %+v", page)
	}
}

func TestPageUsageInflightExtremePageDoesNotOverflow(t *testing.T) {
	page := int(^uint(0) >> 1)
	got, total := PageUsageInflight(nil, page, 1)
	if len(got) != 0 || total != 0 {
		t.Fatalf("got=%v total=%d", got, total)
	}
}

func TestUsageInflightBillingIdentityDeduplicatesCommittedRows(t *testing.T) {
	for _, key := range []ctxkey.Key{ctxkey.ClientRequestID, ctxkey.RequestID} {
		ctx := context.WithValue(context.Background(), key, "abc")
		id := UsageInflightRequestID(ctx)
		if id != resolveUsageBillingRequestID(ctx, "upstream") {
			t.Fatalf("identity mismatch: %q", id)
		}
		got := filterUsageInflight([]UsageInflightSnapshot{{RequestID: id}}, usagestats.UsageLogFilters{}, map[string]struct{}{id: {}})
		if len(got) != 0 {
			t.Fatalf("committed request still live: %+v", got)
		}
	}
}

func TestFilterUsageInflightMatchesCompleteRequestID(t *testing.T) {
	rows := []UsageInflightSnapshot{{RequestID: "client:fcfb9013-144a-4994-a2e7-f40eb688adc5"}}
	for _, id := range []string{"fcfb9013-144a-4994-a2e7-f40eb688adc5", rows[0].RequestID} {
		got := filterUsageInflight(rows, usagestats.UsageLogFilters{RequestID: id}, nil)
		if len(got) != 1 {
			t.Fatalf("complete identity should match, got %+v", got)
		}
	}
	for _, id := range []string{"fcfb9013", "%", "FCFB9013-144a-4994-a2e7-f40eb688adc5"} {
		if got := filterUsageInflight(rows, usagestats.UsageLogFilters{RequestID: id}, nil); len(got) != 0 {
			t.Fatalf("fragment/wildcard/case change should not match, got %+v", got)
		}
	}
}
