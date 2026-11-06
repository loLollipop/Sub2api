package service

import "strings"

const (
	emailBrandAccent = "#b64e16"
	emailAlertAccent = "#b42318"
)

// emailDocument provides a self-contained, table-based email layout. All inputs
// are trusted HTML: callers must escape user data or pass template placeholders
// that will be escaped by renderNotificationEmail before delivery.
func emailDocument(locale, siteName, accent, title, content string) string {
	lang, label := "en", "SERVICE NOTIFICATION"
	footer := "This email was sent automatically by " + siteName + ". Please do not reply directly."
	if siteName == "" {
		footer = "This is an automated message. Please do not reply directly."
	}
	if normalizeNotificationLocale(locale) == notificationEmailLocaleChinese {
		lang, label = "zh-CN", "服务通知"
		footer = "此邮件由 " + siteName + " 自动发送，请勿直接回复。"
	}
	brand := ""
	if siteName != "" {
		brand = `<p style="margin:0 0 6px;font-size:19px;line-height:1.5;font-weight:700;color:#302a23;">` + siteName + `</p>`
	}
	return `<!DOCTYPE html>
<html lang="` + lang + `">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <meta name="color-scheme" content="light">
  <title>` + title + `</title>
  <style>
    @media only screen and (max-width: 620px) {
      .email-page { padding: 20px 12px !important; }
      .email-header { padding: 24px 24px 20px !important; }
      .email-content { padding: 24px !important; }
      .email-title { font-size: 24px !important; }
      .email-code { font-size: 30px !important; letter-spacing: 6px !important; }
    }
  </style>
</head>
<body style="margin:0;padding:0;background-color:#f5f2ec;color:#5e574e;font-family:'Segoe UI','PingFang SC','Microsoft YaHei',sans-serif;-webkit-text-size-adjust:100%;">
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="width:100%;background-color:#f5f2ec;color:#5e574e;font-family:'Segoe UI','PingFang SC','Microsoft YaHei',sans-serif;"><tr><td class="email-page" align="center" style="padding:40px 16px;">
    <!--[if mso]><table role="presentation" width="600" align="center"><tr><td><![endif]-->
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="width:100%;max-width:600px;text-align:left;background-color:#ffffff;border:1px solid #e8e1d7;border-top:4px solid ` + accent + `;border-radius:16px;border-spacing:0;">
      <tr><td class="email-header" style="padding:28px 36px 24px;border-bottom:1px solid #eee8df;overflow-wrap:anywhere;word-break:break-word;">
        ` + brand + `
        <p style="margin:0;font-size:11px;line-height:1.5;letter-spacing:2px;color:` + accent + `;font-weight:600;">` + label + `</p>
      </td></tr>
      <tr><td class="email-content" style="padding:30px 36px 32px;font-size:15px;line-height:1.8;overflow-wrap:anywhere;word-break:break-word;">
        <h1 class="email-title" style="margin:0 0 24px;color:#302a23;font-size:28px;line-height:1.35;letter-spacing:-0.5px;font-weight:700;">` + title + `</h1>
        ` + content + `
      </td></tr>
    </table>
    <!--[if mso]></td></tr></table><![endif]-->
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" border="0" style="width:100%;max-width:600px;"><tr><td align="center" style="padding:20px 20px 0;font-size:12px;line-height:1.8;color:#857b6e;overflow-wrap:anywhere;word-break:break-word;">` + footer + `</td></tr></table>
  </td></tr></table>
</body>
</html>`
}

func emailCodeBlock(code string) string {
	return `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="width:100%;margin:24px 0;background-color:#fcf6ed;border:1px solid #efdfc8;border-radius:12px;"><tr><td class="email-code" align="center" style="padding:20px 12px;color:#a94412;font-family:Consolas,'Courier New',monospace;font-size:34px;line-height:1.4;font-weight:700;letter-spacing:8px;">` + code + `</td></tr></table>`
}

func emailAction(url, label string) string {
	return `<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:24px 0;"><tr><td align="center" bgcolor="#b64e16" style="background-color:#b64e16;border-radius:8px;mso-padding-alt:12px 24px;"><a href="` + url + `" style="display:inline-block;background-color:#b64e16;border:1px solid #b64e16;border-radius:8px;padding:12px 24px;color:#ffffff;font-size:15px;line-height:1.5;font-weight:600;text-decoration:none;text-align:center;">` + label + `</a></td></tr></table>`
}

func emailMetricBlock(label, value string) string {
	return `<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="width:100%;margin:24px 0;background-color:#fcf6ed;border:1px solid #efdfc8;border-radius:12px;"><tr><td style="padding:20px 24px;"><p style="margin:0 0 6px;color:#7b6851;font-size:13px;">` + label + `</p><p style="margin:0;color:#a94412;font-size:36px;line-height:1.3;font-weight:700;">` + value + `</p></td></tr></table>`
}

func emailDetails(rows ...[2]string) string {
	var b strings.Builder
	b.WriteString(`<table width="100%" cellpadding="0" cellspacing="0" style="width:100%;margin:20px 0;border-collapse:collapse;table-layout:fixed;font-size:14px;line-height:1.7;">`)
	for _, row := range rows {
		b.WriteString(`<tr><td style="width:40%;padding:12px 8px 12px 0;border-bottom:1px solid #eee8df;color:#857565;vertical-align:top;overflow-wrap:anywhere;word-break:break-word;">` + row[0] + `</td><td style="padding:12px 0 12px 8px;border-bottom:1px solid #eee8df;color:#302a23;vertical-align:top;overflow-wrap:anywhere;word-break:break-word;white-space:pre-wrap;">` + row[1] + `</td></tr>`)
	}
	b.WriteString(`</table>`)
	return b.String()
}
