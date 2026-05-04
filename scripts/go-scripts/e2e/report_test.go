package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteE2EReportInterleavesTextAndImages 验证报告按事件顺序穿插文字和图片。
func TestWriteE2EReportInterleavesTextAndImages(t *testing.T) {
	outputDir := t.TempDir()
	events := []reportEvent{
		reportStep("第一段说明。"),
		reportImage("第一张图", "screenshots/01-first.png"),
		reportStep("第二段说明。"),
		reportImage("第二张图", "screenshots/02-second.png"),
	}

	writeE2EReport(outputDir, "顺序测试报告", true, events)

	data, err := os.ReadFile(filepath.Join(outputDir, "report.md"))
	if err != nil {
		t.Fatalf("读取报告失败: %v", err)
	}
	content := string(data)
	assertReportOrder(t, content, []string{
		"第一段说明。",
		"![第一张图](screenshots/01-first.png)",
		"第二段说明。",
		"![第二张图](screenshots/02-second.png)",
	})
}

// assertReportOrder 使用 content 和 fragments 参数断言片段按顺序出现。
func assertReportOrder(t *testing.T, content string, fragments []string) {
	t.Helper()
	offset := 0
	for _, fragment := range fragments {
		index := strings.Index(content[offset:], fragment)
		if index < 0 {
			t.Fatalf("报告缺少顺序片段 %q，当前报告:\n%s", fragment, content)
		}
		offset += index + len(fragment)
	}
}
