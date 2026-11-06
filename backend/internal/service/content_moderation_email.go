package service

import (
	"fmt"
	"html"
	"strings"
	"time"
)

func buildContentModerationViolationEmailBody(siteName string, log *ContentModerationLog, cfg *ContentModerationConfig) string {
	if log == nil {
		return ""
	}
	userName := strings.TrimSpace(log.UserEmail)
	if userName == "" && log.UserID != nil {
		userName = fmt.Sprintf("UID %d", *log.UserID)
	}
	threshold := cfg.BanThreshold
	if threshold <= 0 {
		threshold = defaultContentModerationBanThreshold
	}
	content := `<p style="margin:0 0 16px;">尊敬的用户 <strong>` + html.EscapeString(userName) + `</strong>，您的 API 请求在内容审计中触发平台风控策略。详情如下。</p>
<h2 style="margin:24px 0 0;color:#b42318;font-size:16px;">触发详情</h2>` + emailDetails(
		[2]string{"触发时间", time.Now().Format("2006-01-02 15:04:05")},
		[2]string{"触发来源", "内容审核"},
		[2]string{"所属分组", html.EscapeString(defaultContentModerationString(log.GroupName, "-"))},
		[2]string{"命中类别", fmt.Sprintf("%s / %.3f", html.EscapeString(defaultContentModerationString(log.HighestCategory, "-")), log.HighestScore)},
		[2]string{"累计触发次数", fmt.Sprintf("%d 次（阈值 %d）", log.ViolationCount, threshold)},
	)
	if log.AutoBanned {
		content += contentModerationEmailBanStatus
	}
	return emailDocument("zh", html.EscapeString(siteName), emailAlertAccent, "账户触发内容审计规则", content)
}

const contentModerationEmailBanStatus = `<p style="margin:24px 0 0;padding:16px 20px;background-color:#fff1ef;border:1px solid #f5cdc7;border-radius:10px;color:#b42318;font-weight:600;">账户当前处于封禁状态，所有 API 请求将被拒绝</p>`

func buildContentModerationAccountDisabledEmailBody(siteName string, log *ContentModerationLog, cfg *ContentModerationConfig) string {
	if log == nil {
		return ""
	}
	userName := strings.TrimSpace(log.UserEmail)
	if userName == "" && log.UserID != nil {
		userName = fmt.Sprintf("UID %d", *log.UserID)
	}
	threshold := cfg.BanThreshold
	if threshold <= 0 {
		threshold = defaultContentModerationBanThreshold
	}
	content := `<p style="margin:0 0 16px;">尊敬的用户 <strong>` + html.EscapeString(userName) + `</strong>，您的账户在计数周期内多次触发平台风控策略，系统已自动禁用该账户。详情如下。</p>
<h2 style="margin:24px 0 0;color:#b42318;font-size:16px;">封禁详情</h2>` + emailDetails(
		[2]string{"封禁时间", time.Now().Format("2006-01-02 15:04:05")},
		[2]string{"触发来源", "内容审核"},
		[2]string{"所属分组", html.EscapeString(defaultContentModerationString(log.GroupName, "-"))},
		[2]string{"命中类别", fmt.Sprintf("%s / %.3f", html.EscapeString(defaultContentModerationString(log.HighestCategory, "-")), log.HighestScore)},
		[2]string{"累计触发次数", fmt.Sprintf("%d 次（阈值 %d）", log.ViolationCount, threshold)},
	) + contentModerationEmailBanStatus + `<p style="margin:24px 0 0;">如需申诉或恢复账号，请联系平台管理员处理。</p>`
	return emailDocument("zh", html.EscapeString(siteName), emailAlertAccent, "账户已被自动禁用", content)
}

func defaultContentModerationString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

// buildCyberPolicyNoticeEmailBody 是 cyber_policy 通知邮件的内置兜底正文，
// 当 notification email 模板渲染失败时使用（与 sendViolationEmail 的兜底同理）。
func buildCyberPolicyNoticeEmailBody(siteName string, log *ContentModerationLog) string {
	if log == nil {
		return ""
	}
	userName := strings.TrimSpace(log.UserEmail)
	if userName == "" && log.UserID != nil {
		userName = fmt.Sprintf("UID %d", *log.UserID)
	}
	content := `<p style="margin:0 0 16px;">尊敬的用户 <strong>` + html.EscapeString(userName) + `</strong>，您的请求被上游网络安全策略（cyber policy）拦截。</p>` + emailDetails(
		[2]string{"触发时间", log.CreatedAt.Format("2006-01-02 15:04:05")},
		[2]string{"模型", html.EscapeString(defaultContentModerationString(log.Model, "-"))},
		[2]string{"上游说明", `<span style="word-break:break-all;">` + html.EscapeString(defaultContentModerationString(log.Error, "-")) + `</span>`},
	) + `<p style="margin:24px 0 0;">如认为系误判，可调整请求措辞后重试，或申请获得授权的安全访问权限。</p>`
	return emailDocument("zh", html.EscapeString(siteName), emailAlertAccent, "请求被网络安全策略拦截", content)
}
