package cireport

import (
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"strings"
)

var markdownImagePattern = regexp.MustCompile(`^!\[(.*)]\((.*)\)$`)
var markdownLinkPattern = regexp.MustCompile(`\[(.*?)]\((.*?)\)`)

// renderSummaryMarkdown 使用 meta 参数生成 GitHub Workflow Summary 内容。
func renderSummaryMarkdown(meta ReportMeta) string {
	status := "失败"
	if meta.Passed {
		status = "通过"
	}
	lines := []string{
		"# E2E 测试概要",
		"",
		fmt.Sprintf("- 结果: %s", status),
		fmt.Sprintf("- 提交: `%s`", meta.SHA),
		fmt.Sprintf("- 分支/标签: `%s`", meta.RefName),
	}
	if meta.RunURL != "" {
		lines = append(lines, fmt.Sprintf("- Workflow: %s", meta.RunURL))
	}
	if meta.PagesURL != "" {
		lines = append(lines, fmt.Sprintf("- Pages 报告: %s", meta.PagesURL))
	}
	lines = append(lines,
		"",
		"| 用例 | 结果 | 报告 |",
		"| --- | --- | --- |",
	)
	for _, item := range meta.Cases {
		caseStatus := "失败"
		if item.Passed {
			caseStatus = "通过"
		}
		report := item.ReportURL
		if meta.PagesURL != "" {
			report = strings.TrimRight(meta.PagesURL, "/") + "/" + item.ReportURL
		}
		lines = append(lines, fmt.Sprintf("| `%s` | %s | [查看](%s) |", item.Name, caseStatus, report))
	}
	return strings.Join(lines, "\n") + "\n"
}

// renderReportIndexHTML 使用 meta 参数生成当前提交报告首页。
func renderReportIndexHTML(meta ReportMeta) string {
	statusClass := "failed"
	statusText := "失败"
	if meta.Passed {
		statusClass = "passed"
		statusText = "通过"
	}
	var rows strings.Builder
	for _, item := range meta.Cases {
		caseStatusClass := "failed"
		caseStatusText := "失败"
		if item.Passed {
			caseStatusClass = "passed"
			caseStatusText = "通过"
		}
		rows.WriteString("<tr><td><code>")
		rows.WriteString(html.EscapeString(item.Name))
		rows.WriteString("</code></td><td><span class=\"status ")
		rows.WriteString(caseStatusClass)
		rows.WriteString("\">")
		rows.WriteString(caseStatusText)
		rows.WriteString("</span></td><td><a href=\"")
		rows.WriteString(html.EscapeString(item.ReportURL))
		rows.WriteString("\">查看报告</a></td></tr>\n")
	}
	body := fmt.Sprintf(`
<main>
  <p class="eyebrow">AgentHub E2E</p>
  <h1>提交 %s 测试报告</h1>
  <section class="meta">
    <div><span>结果</span><strong class="status %s">%s</strong></div>
    <div><span>分支/标签</span><strong>%s</strong></div>
    <div><span>完整提交</span><code>%s</code></div>
    <div><span>生成时间</span><strong>%s</strong></div>
  </section>
  <section>
    <h2>用例</h2>
    <table>
      <thead><tr><th>用例</th><th>结果</th><th>报告</th></tr></thead>
      <tbody>%s</tbody>
    </table>
  </section>
</main>`,
		html.EscapeString(meta.ShortSHA),
		statusClass,
		statusText,
		html.EscapeString(meta.RefName),
		html.EscapeString(meta.SHA),
		html.EscapeString(meta.CreatedAt),
		rows.String(),
	)
	return renderHTMLDocument("AgentHub E2E 报告 "+meta.ShortSHA, body)
}

// renderMarkdownPage 使用 content 参数把简化 Markdown 报告渲染为 HTML 页面。
func renderMarkdownPage(content string) string {
	var body strings.Builder
	body.WriteString("<main class=\"markdown\">\n")
	for _, line := range strings.Split(content, "\n") {
		body.WriteString(renderMarkdownLine(line))
		body.WriteByte('\n')
	}
	body.WriteString("</main>\n")
	return renderHTMLDocument("AgentHub E2E 用例报告", body.String())
}

// renderMarkdownLine 使用 line 参数渲染单行简化 Markdown。
func renderMarkdownLine(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "<br>"
	}
	if strings.HasPrefix(trimmed, "# ") {
		return "<h1>" + html.EscapeString(strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))) + "</h1>"
	}
	if strings.HasPrefix(trimmed, "## ") {
		return "<h2>" + html.EscapeString(strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))) + "</h2>"
	}
	if matches := markdownImagePattern.FindStringSubmatch(trimmed); matches != nil {
		return "<figure><img src=\"" + html.EscapeString(filepath.ToSlash(matches[2])) + "\" alt=\"" + html.EscapeString(matches[1]) + "\"><figcaption>" + html.EscapeString(matches[1]) + "</figcaption></figure>"
	}
	if strings.HasPrefix(trimmed, "- ") {
		return "<p class=\"bullet\">• " + renderInlineMarkdown(strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))) + "</p>"
	}
	return "<p>" + renderInlineMarkdown(trimmed) + "</p>"
}

// renderInlineMarkdown 使用 text 参数渲染行内链接。
func renderInlineMarkdown(text string) string {
	escaped := html.EscapeString(text)
	return markdownLinkPattern.ReplaceAllString(escaped, `<a href="$2">$1</a>`)
}

// renderHTMLDocument 使用 title 和 body 参数生成完整 HTML 文档。
func renderHTMLDocument(title string, body string) string {
	return "<!doctype html>\n<html lang=\"zh-CN\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n<title>" + html.EscapeString(title) + "</title>\n<style>\n" + pageCSS() + "\n</style>\n</head>\n<body>\n" + body + "\n</body>\n</html>\n"
}

// pageCSS 返回报告页面共享样式。
func pageCSS() string {
	return `:root{color-scheme:light;--bg:#f6f7f9;--panel:#fff;--text:#202124;--muted:#5f6368;--line:#dde1e6;--ok:#137333;--bad:#b3261e;--link:#0b57d0}body{margin:0;background:var(--bg);color:var(--text);font:14px/1.6 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}main{max-width:1080px;margin:0 auto;padding:32px 20px 56px}.eyebrow{margin:0 0 6px;color:var(--muted);font-size:12px;text-transform:uppercase;letter-spacing:.08em}h1{margin:0 0 20px;font-size:28px;line-height:1.25}h2{margin:28px 0 12px;font-size:18px}.meta{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px;margin:0 0 24px}.meta div,table,.markdown{background:var(--panel);border:1px solid var(--line);border-radius:8px}.meta div{padding:14px}.meta span{display:block;color:var(--muted);font-size:12px}.meta strong,.meta code{display:block;margin-top:4px;word-break:break-all}table{width:100%;border-collapse:collapse;overflow:hidden}th,td{padding:10px 12px;border-bottom:1px solid var(--line);text-align:left}tr:last-child td{border-bottom:0}.status{font-weight:700}.status.passed{color:var(--ok)}.status.failed{color:var(--bad)}a{color:var(--link);text-decoration:none}a:hover{text-decoration:underline}code{font-family:"SFMono-Regular",Consolas,monospace}.markdown{padding:22px}.markdown h1{font-size:24px}.markdown p{margin:8px 0}.bullet{padding-left:4px}figure{margin:18px 0}img{max-width:100%;border:1px solid var(--line);border-radius:6px;background:#fff}figcaption{margin-top:6px;color:var(--muted);font-size:12px}`
}
