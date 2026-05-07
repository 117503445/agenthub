package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// runPWACase 使用 ctx 参数验证 PWA manifest 和 service worker。
func runPWACase(ctx E2EContext) (success bool) {
	events := make([]reportEvent, 0)
	defer func() {
		writeE2EReport(ctx.OutputDir, "PWA E2E 测试报告", success, events)
	}()

	session, err := newBrowserSession(1280, 820)
	if err != nil {
		ctx.Logger.Errorf("创建浏览器失败: %v", err)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		return false
	}
	defer session.Close()
	page := session.page

	fail := func(err error) bool {
		ctx.Logger.Errorf("PWA E2E 失败: %v", err)
		screenshot(page, filepath.Join(ctx.ScreenshotsDir, "failed.png"), true)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		events = append(events, reportImage("失败现场", "screenshots/failed.png"))
		return false
	}

	manifest, err := fetchPWAManifest(ctx.BaseURL)
	if err != nil {
		return fail(err)
	}
	if manifest["name"] != "AgentHub" || manifest["display"] != "standalone" {
		return fail(fmt.Errorf("manifest 内容不正确: %#v", manifest))
	}
	if err := fetchPWAAsset(ctx.BaseURL + "/sw.js"); err != nil {
		return fail(err)
	}
	if err := gotoPage(page, ctx.BaseURL); err != nil {
		return fail(err)
	}
	if err := expectPageState(page, "等待 PWA manifest 链接", `async () => {
		const link = document.querySelector('link[rel="manifest"]');
		if (!link) {
			return 'missing manifest link';
		}
		const response = await fetch(new URL(link.getAttribute('href'), window.location.href).toString());
		if (!response.ok) {
			return 'manifest status=' + response.status;
		}
		const manifest = await response.json();
		return manifest.name === 'AgentHub' && manifest.display === 'standalone' ? '' : JSON.stringify(manifest);
	}`, nil, 10*time.Second); err != nil {
		return fail(err)
	}
	if err := expectPageState(page, "等待 service worker 注册", `async () => {
		if (!('serviceWorker' in navigator)) {
			return 'service worker unsupported';
		}
		const registration = await navigator.serviceWorker.ready;
		return registration?.active ? '' : 'missing active registration';
	}`, nil, 15*time.Second); err != nil {
		return fail(err)
	}

	events = append(events, reportStep("页面声明 PWA manifest，浏览器可注册 AgentHub service worker。"))
	return true
}

// fetchPWAManifest 使用 baseURL 参数读取并解析 manifest。
func fetchPWAManifest(baseURL string) (map[string]any, error) {
	response, err := http.Get(strings.TrimRight(baseURL, "/") + "/manifest.webmanifest")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest 状态码不正确: %d", response.StatusCode)
	}
	var manifest map[string]any
	if err := json.NewDecoder(response.Body).Decode(&manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

// fetchPWAAsset 使用 url 参数确认 PWA 静态资源可访问。
func fetchPWAAsset(url string) error {
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("PWA 资源状态码不正确: %s status=%d", url, response.StatusCode)
	}
	return nil
}
