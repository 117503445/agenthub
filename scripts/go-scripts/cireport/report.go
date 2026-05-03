// Package cireport 生成 CI E2E 报告和 GitHub Pages 索引。
package cireport

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/117503445/agenthub/scripts/go-scripts/common"
)

// CaseResult 表示单个 E2E 用例的报告结果。
type CaseResult struct {
	Name      string `json:"name"`      // Name 表示用例名称。
	Passed    bool   `json:"passed"`    // Passed 表示用例是否通过。
	ReportURL string `json:"reportURL"` // ReportURL 表示当前用例 HTML 报告相对路径。
}

// ReportMeta 表示一次 CI E2E 报告的元数据。
type ReportMeta struct {
	SHA       string       `json:"sha"`       // SHA 表示当前提交完整哈希。
	ShortSHA  string       `json:"shortSHA"`  // ShortSHA 表示当前提交短哈希。
	RefName   string       `json:"refName"`   // RefName 表示当前分支或标签名。
	RunURL    string       `json:"runURL"`    // RunURL 表示 GitHub Actions 运行链接。
	PagesURL  string       `json:"pagesURL"`  // PagesURL 表示当前报告页面链接。
	Passed    bool         `json:"passed"`    // Passed 表示本次 E2E 是否全部通过。
	ExitCode  int          `json:"exitCode"`  // ExitCode 表示 E2E 命令退出码。
	CreatedAt string       `json:"createdAt"` // CreatedAt 表示报告生成时间。
	Cases     []CaseResult `json:"cases"`     // Cases 表示全部用例结果。
}

// ReportOptions 表示生成当前 CI 报告所需参数。
type ReportOptions struct {
	InputDir    string // InputDir 表示 E2E 原始输出目录。
	OutputDir   string // OutputDir 表示当前报告输出目录。
	SHA         string // SHA 表示当前提交完整哈希。
	RefName     string // RefName 表示当前分支或标签名。
	RunURL      string // RunURL 表示 GitHub Actions 运行链接。
	PagesURL    string // PagesURL 表示当前报告页面链接。
	E2EExitCode int    // E2EExitCode 表示 E2E 命令退出码。
}

// RunReport 使用 args 参数解析命令行并生成当前 CI 报告。
func RunReport(args []string) error {
	flags := flag.NewFlagSet("ci-report", flag.ContinueOnError)
	options := ReportOptions{}
	flags.StringVar(&options.InputDir, "input", filepath.Join("data", "e2e"), "E2E 原始输出目录")
	flags.StringVar(&options.OutputDir, "output", filepath.Join("data", "ci-report"), "CI 报告输出目录")
	flags.StringVar(&options.SHA, "sha", "", "当前提交哈希")
	flags.StringVar(&options.RefName, "ref", "", "当前分支或标签名")
	flags.StringVar(&options.RunURL, "run-url", "", "GitHub Actions 运行链接")
	flags.StringVar(&options.PagesURL, "pages-url", "", "GitHub Pages 报告链接")
	flags.IntVar(&options.E2EExitCode, "e2e-exit-code", 0, "E2E 命令退出码")
	if err := flags.Parse(args); err != nil {
		return err
	}
	return GenerateReport(options)
}

// GenerateReport 使用 options 参数生成当前 CI 报告目录。
func GenerateReport(options ReportOptions) error {
	if options.SHA == "" {
		return fmt.Errorf("sha 不能为空")
	}
	if options.RefName == "" {
		options.RefName = "unknown"
	}
	if err := os.RemoveAll(options.OutputDir); err != nil {
		return fmt.Errorf("清理 CI 报告目录失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(options.OutputDir, "cases"), 0755); err != nil {
		return fmt.Errorf("创建 CI 报告目录失败: %w", err)
	}

	cases, err := collectCaseResults(options.InputDir)
	if err != nil {
		return err
	}
	if err := copyCaseReportArtifacts(options.InputDir, filepath.Join(options.OutputDir, "cases"), cases); err != nil {
		return err
	}
	for _, item := range cases {
		if err := writeCaseHTML(filepath.Join(options.OutputDir, "cases", item.Name)); err != nil {
			return err
		}
	}

	meta := ReportMeta{
		SHA:       options.SHA,
		ShortSHA:  shortSHA(options.SHA),
		RefName:   options.RefName,
		RunURL:    options.RunURL,
		PagesURL:  options.PagesURL,
		Passed:    options.E2EExitCode == 0 && allCasesPassed(cases),
		ExitCode:  options.E2EExitCode,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Cases:     cases,
	}
	if err := writeJSON(filepath.Join(options.OutputDir, "meta.json"), meta); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(options.OutputDir, "summary.md"), []byte(renderSummaryMarkdown(meta)), 0644); err != nil {
		return fmt.Errorf("写入 CI 摘要失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(options.OutputDir, "index.html"), []byte(renderReportIndexHTML(meta)), 0644); err != nil {
		return fmt.Errorf("写入 CI 报告首页失败: %w", err)
	}
	return nil
}

// collectCaseResults 使用 inputDir 参数收集全部 E2E 用例结果。
func collectCaseResults(inputDir string) ([]CaseResult, error) {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 E2E 输出目录失败: %w", err)
	}
	results := make([]CaseResult, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		reportPath := filepath.Join(inputDir, name, "report.md")
		if _, err := os.Stat(reportPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("读取用例报告状态失败 %s: %w", name, err)
		}
		passed := false
		if data, err := os.ReadFile(reportPath); err == nil {
			passed = strings.Contains(string(data), "- 结果: 通过")
		}
		results = append(results, CaseResult{
			Name:      name,
			Passed:    passed,
			ReportURL: filepath.ToSlash(filepath.Join("cases", name, "report.html")),
		})
	}
	sort.Slice(results, func(i int, j int) bool {
		return results[i].Name < results[j].Name
	})
	return results, nil
}

// copyCaseReportArtifacts 使用 inputDir、outputDir 和 cases 参数复制 E2E 报告资源。
func copyCaseReportArtifacts(inputDir string, outputDir string, cases []CaseResult) error {
	for _, item := range cases {
		src := filepath.Join(inputDir, item.Name)
		dst := filepath.Join(outputDir, item.Name)
		if err := os.MkdirAll(dst, 0755); err != nil {
			return fmt.Errorf("创建用例报告目录失败 %s: %w", item.Name, err)
		}
		if err := copyFile(filepath.Join(src, "report.md"), filepath.Join(dst, "report.md")); err != nil {
			return fmt.Errorf("复制用例报告失败 %s: %w", item.Name, err)
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return fmt.Errorf("读取用例报告资源失败 %s: %w", item.Name, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || !isReportAssetDir(entry.Name()) {
				continue
			}
			if err := common.CopyDir(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return fmt.Errorf("复制用例报告资源失败 %s/%s: %w", item.Name, entry.Name(), err)
			}
		}
	}
	return nil
}

// isReportAssetDir 使用 name 参数判断目录是否属于报告资源。
func isReportAssetDir(name string) bool {
	return name == "logs" || name == "screenshots" || strings.HasSuffix(name, "-logs")
}

// copyFile 使用 src 和 dst 参数复制单个文件。
func copyFile(src string, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// writeCaseHTML 使用 caseDir 参数为单个用例写入 HTML 报告。
func writeCaseHTML(caseDir string) error {
	data, err := os.ReadFile(filepath.Join(caseDir, "report.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取用例 Markdown 报告失败: %w", err)
	}
	html := renderMarkdownPage(string(data))
	if err := os.WriteFile(filepath.Join(caseDir, "report.html"), []byte(html), 0644); err != nil {
		return fmt.Errorf("写入用例 HTML 报告失败: %w", err)
	}
	return nil
}

// allCasesPassed 使用 cases 参数判断全部用例是否通过。
func allCasesPassed(cases []CaseResult) bool {
	if len(cases) == 0 {
		return false
	}
	for _, item := range cases {
		if !item.Passed {
			return false
		}
	}
	return true
}

// writeJSON 使用 path 和 value 参数写入格式化 JSON 文件。
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 JSON 失败: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("写入 JSON 文件失败: %w", err)
	}
	return nil
}

// shortSHA 使用 sha 参数返回短提交哈希。
func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
