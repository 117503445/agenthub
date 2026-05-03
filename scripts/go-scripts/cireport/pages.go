package cireport

import (
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PagesIndexOptions 表示生成 GitHub Pages 首页所需参数。
type PagesIndexOptions struct {
	SiteDir string // SiteDir 表示 GitHub Pages 站点目录。
}

// RunPagesIndex 使用 args 参数解析命令行并生成 GitHub Pages 首页。
func RunPagesIndex(args []string) error {
	flags := flag.NewFlagSet("ci-pages-index", flag.ContinueOnError)
	options := PagesIndexOptions{}
	flags.StringVar(&options.SiteDir, "site-dir", "pages-site", "GitHub Pages 站点目录")
	if err := flags.Parse(args); err != nil {
		return err
	}
	return GeneratePagesIndex(options)
}

// GeneratePagesIndex 使用 options 参数生成 GitHub Pages 首页。
func GeneratePagesIndex(options PagesIndexOptions) error {
	reportsDir := filepath.Join(options.SiteDir, "reports")
	metas, err := collectReportMetas(reportsDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(options.SiteDir, 0755); err != nil {
		return fmt.Errorf("创建 Pages 目录失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(options.SiteDir, ".nojekyll"), []byte{}, 0644); err != nil {
		return fmt.Errorf("写入 .nojekyll 失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(options.SiteDir, "index.html"), []byte(renderPagesIndexHTML(metas)), 0644); err != nil {
		return fmt.Errorf("写入 Pages 首页失败: %w", err)
	}
	return nil
}

// collectReportMetas 使用 reportsDir 参数读取全部提交报告元数据。
func collectReportMetas(reportsDir string) ([]ReportMeta, error) {
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 Pages 报告目录失败: %w", err)
	}
	metas := make([]ReportMeta, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(reportsDir, entry.Name(), "meta.json"))
		if err != nil {
			continue
		}
		var meta ReportMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		metas = append(metas, meta)
	}
	sort.Slice(metas, func(i int, j int) bool {
		return metas[i].CreatedAt > metas[j].CreatedAt
	})
	return metas, nil
}

// renderPagesIndexHTML 使用 metas 参数生成全部提交报告索引页。
func renderPagesIndexHTML(metas []ReportMeta) string {
	var rows strings.Builder
	for _, meta := range metas {
		statusClass := "failed"
		statusText := "失败"
		if meta.Passed {
			statusClass = "passed"
			statusText = "通过"
		}
		rows.WriteString("<tr><td><a href=\"reports/")
		rows.WriteString(html.EscapeString(meta.SHA))
		rows.WriteString("/\">")
		rows.WriteString(html.EscapeString(meta.ShortSHA))
		rows.WriteString("</a></td><td><span class=\"status ")
		rows.WriteString(statusClass)
		rows.WriteString("\">")
		rows.WriteString(statusText)
		rows.WriteString("</span></td><td>")
		rows.WriteString(html.EscapeString(meta.RefName))
		rows.WriteString("</td><td>")
		rows.WriteString(html.EscapeString(meta.CreatedAt))
		rows.WriteString("</td></tr>\n")
	}
	body := `<main>
  <p class="eyebrow">AgentHub E2E</p>
  <h1>测试报告索引</h1>
  <section>
    <table>
      <thead><tr><th>提交</th><th>结果</th><th>分支/标签</th><th>生成时间</th></tr></thead>
      <tbody>` + rows.String() + `</tbody>
    </table>
  </section>
</main>`
	return renderHTMLDocument("AgentHub E2E 报告索引", body)
}
