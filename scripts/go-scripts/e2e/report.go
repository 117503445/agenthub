package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
)

type reportEventKind string

const (
	reportEventStep  reportEventKind = "step"
	reportEventImage reportEventKind = "image"
)

// reportEvent 表示 E2E 报告中的一个有序事件。
type reportEvent struct {
	Kind  reportEventKind // Kind 表示事件类型。
	Text  string          // Text 表示步骤说明或图片标题。
	Image string          // Image 表示图片相对路径。
}

// reportStep 使用 text 参数创建文字步骤事件。
func reportStep(text string) reportEvent {
	return reportEvent{Kind: reportEventStep, Text: text}
}

// reportImage 使用 alt 和 path 参数创建图片事件。
func reportImage(alt string, path string) reportEvent {
	return reportEvent{Kind: reportEventImage, Text: alt, Image: path}
}

// writeE2EReport 使用 outputDir、title、success 和 events 参数写入 Markdown 报告。
func writeE2EReport(outputDir string, title string, success bool, events []reportEvent) {
	report := []string{
		"# " + title,
		"",
		fmt.Sprintf("- 结果: %s", passText(success)),
		"- 日志: [test.log](logs/test.log)",
		"- 服务日志: [server.log](logs/server.log)",
		"",
		"## 过程",
		"",
	}
	report = appendReportEvents(report, events)
	writeFile(filepath.Join(outputDir, "report.md"), strings.Join(report, "\n")+"\n")
}

// appendReportEvents 使用 report 和 events 参数追加有序报告事件。
func appendReportEvents(report []string, events []reportEvent) []string {
	for _, event := range events {
		switch event.Kind {
		case reportEventImage:
			report = append(report, "", fmt.Sprintf("![%s](%s)", event.Text, event.Image), "")
		default:
			report = append(report, "- "+event.Text)
		}
	}
	return report
}
