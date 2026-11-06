package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOfficialEmailTemplatesRenderBothLocales(t *testing.T) {
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	for _, event := range notificationEmailEventOrder {
		for _, locale := range []string{"en", "zh"} {
			t.Run(event+"/"+locale, func(t *testing.T) {
				tmpl, err := svc.GetTemplate(context.Background(), event, locale)
				require.NoError(t, err)
				require.NoError(t, validateNotificationEmailTemplate(event, tmpl.Subject, tmpl.HTML))
				preview, err := svc.PreviewTemplate(context.Background(), NotificationEmailPreviewInput{
					Event: event, Locale: locale,
					Variables: map[string]string{"site_name": "My & Site", "recipient_name": "<Alice>"},
				})
				require.NoError(t, err)
				require.Empty(t, notificationEmailPlaceholdersIn(preview.HTML))
				require.Contains(t, preview.HTML, "My &amp; Site")
				require.NotContains(t, preview.HTML, "<Alice>")
				require.NotContains(t, preview.HTML, "%!")
				require.Contains(t, preview.HTML, `role="presentation"`)
				require.NotContains(t, preview.HTML, "linear-gradient")
				if locale == "zh" {
					require.Contains(t, preview.HTML, `<html lang="zh-CN">`)
					require.Contains(t, preview.HTML, "自动发送，请勿直接回复。")
					require.NotContains(t, preview.HTML, "Please do not reply directly.")
				} else {
					require.Contains(t, preview.HTML, `<html lang="en">`)
					require.Contains(t, preview.HTML, "Please do not reply directly.")
				}
			})
		}
	}
}

func TestStyledEmailPreservesActionsAndAmounts(t *testing.T) {
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	for _, locale := range []string{"en", "zh"} {
		reset, err := svc.PreviewTemplate(context.Background(), NotificationEmailPreviewInput{
			Event: NotificationEmailEventAuthPasswordReset, Locale: locale,
			Variables: map[string]string{
				"reset_url": "https://example.com/reset?token=abc&source=email", "expires_in_minutes": "30",
			},
		})
		require.NoError(t, err)
		require.Contains(t, reset.HTML, `href="https://example.com/reset?token=abc&amp;source=email"`)
		require.Equal(t, 2, strings.Count(reset.HTML, "https://example.com/reset?token=abc&amp;source=email"))
		require.Contains(t, reset.HTML, "<strong>30</strong>")

		balance, err := svc.PreviewTemplate(context.Background(), NotificationEmailPreviewInput{
			Event: NotificationEmailEventBalanceLow, Locale: locale,
			Variables: map[string]string{
				"current_balance": "3.14", "threshold": "10.00",
				"recharge_url": "https://example.com/pay", "unsubscribe_url": "https://example.com/unsubscribe?token=abc",
			},
		})
		require.NoError(t, err)
		require.Contains(t, balance.HTML, "$3.14")
		require.Contains(t, balance.HTML, "$10.00")
		require.Contains(t, balance.HTML, `href="https://example.com/pay"`)
		require.Contains(t, balance.HTML, `href="https://example.com/unsubscribe?token=abc"`)
	}
}

func TestStyledEmailsKeepSavedCustomTemplate(t *testing.T) {
	svc := NewNotificationEmailService(newNotificationEmailMemorySettingRepo(), nil)
	const custom = `<p>Custom verification {{verification_code}}</p>`
	_, err := svc.UpdateTemplate(context.Background(), NotificationEmailEventAuthVerifyCode, "zh", "Custom subject", custom)
	require.NoError(t, err)
	tmpl, err := svc.GetTemplate(context.Background(), NotificationEmailEventAuthVerifyCode, "zh")
	require.NoError(t, err)
	require.True(t, tmpl.IsCustom)
	require.Equal(t, custom, tmpl.HTML)
}

func TestNotificationVerifyFallbackEscapesDynamicValues(t *testing.T) {
	body := buildNotifyVerifyEmailBody(`<script>123456</script>`, `A&B<Site>`)
	require.Contains(t, body, "A&amp;B&lt;Site&gt;")
	require.Contains(t, body, "&lt;script&gt;123456&lt;/script&gt;")
	require.NotContains(t, body, "<script>")
	require.Contains(t, body, "15 分钟")
	require.Contains(t, body, "15 minutes")
}

func TestBalanceFallbackPreservesAlreadyEscapedNames(t *testing.T) {
	svc := &BalanceNotifyService{}
	body := svc.buildBalanceLowEmailBody("Alice &amp; Bob", 3.14, 10, "My &amp; Site", "")
	require.Contains(t, body, "Alice &amp; Bob")
	require.Contains(t, body, "My &amp; Site")
	require.NotContains(t, body, "&amp;amp;")
	body = svc.buildQuotaAlertEmailBody(42, "&lt;account&gt;", "A&amp;B", "daily", 2, 10, 8, "$5.00", "My &amp; Site")
	require.Contains(t, body, "&lt;account&gt;")
	require.Contains(t, body, "A&amp;B")
	require.NotContains(t, body, "&amp;lt;")
	require.NotContains(t, body, "&amp;amp;")
}

func TestModerationEmailFallbackKeepsDetailsAndBanState(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.BanThreshold = 0
	userID := int64(42)
	log := &ContentModerationLog{
		UserID: &userID, GroupName: "<Pro>", HighestCategory: "<policy>",
		HighestScore: 0.926, ViolationCount: 3,
		CreatedAt: time.Date(2026, 11, 6, 12, 30, 0, 0, time.UTC),
		Model:     "<model>", Error: "<upstream>\nDetails",
	}
	body := buildContentModerationViolationEmailBody("A&B", log, cfg)
	require.Contains(t, body, "UID 42")
	require.Contains(t, body, "&lt;Pro&gt;")
	require.Contains(t, body, "&lt;policy&gt; / 0.926")
	require.Contains(t, body, "A&amp;B")
	require.NotContains(t, body, "所有 API 请求将被拒绝")
	log.AutoBanned = true
	body = buildContentModerationViolationEmailBody("A&B", log, cfg)
	require.Contains(t, body, "所有 API 请求将被拒绝")
	disabled := buildContentModerationAccountDisabledEmailBody("A&B", log, cfg)
	require.Contains(t, disabled, "所有 API 请求将被拒绝")
	require.Contains(t, disabled, "请联系平台管理员处理")
	cyber := buildCyberPolicyNoticeEmailBody("A&B", log)
	require.Contains(t, cyber, "2026-11-06 12:30:00")
	require.Contains(t, cyber, "&lt;model&gt;")
	require.Contains(t, cyber, "&lt;upstream&gt;\nDetails")
	require.NotContains(t, cyber, "<upstream>")
	require.Empty(t, buildContentModerationViolationEmailBody("Site", nil, cfg))
	require.Empty(t, buildContentModerationAccountDisabledEmailBody("Site", nil, cfg))
	require.Empty(t, buildCyberPolicyNoticeEmailBody("Site", nil))
}

func TestOpsAlertEmailFallbackKeepsMetricAndEscapedDetails(t *testing.T) {
	rule := &OpsAlertRule{Name: "<rule>", Severity: "P1", MetricType: "error_rate", Operator: ">", Threshold: 5}
	event := &OpsAlertEvent{Status: "firing", FiredAt: time.Date(2026, 11, 6, 12, 30, 0, 0, time.UTC), Description: "<details>"}
	body := buildOpsAlertEmailBody(rule, event)
	require.Contains(t, body, "&lt;rule&gt;")
	require.Contains(t, body, "&lt;details&gt;")
	require.Contains(t, body, "error_rate &gt; - (threshold 5.00)")
	require.Contains(t, body, "2026-11-06T12:30:00Z")
	value, threshold := 12.5, 10.0
	event.MetricValue, event.ThresholdValue = &value, &threshold
	body = buildOpsAlertEmailBody(rule, event)
	require.Contains(t, body, "error_rate &gt; 12.50 (threshold 10.00)")
	require.Empty(t, buildOpsAlertEmailBody(nil, event))
	require.Empty(t, buildOpsAlertEmailBody(rule, nil))
}
