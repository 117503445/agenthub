package e2e

import (
	"fmt"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

const e2ePollInterval = 150 * time.Millisecond

// waitProbe 表示一次条件探测，返回是否满足、当前状态和错误。
type waitProbe func() (bool, string, error)

// waitForCondition 使用 description、timeout 和 probe 参数等待条件满足。
func waitForCondition(description string, timeout time.Duration, probe waitProbe) error {
	deadline := time.Now().Add(timeout)
	var lastState string
	var lastErr error
	for {
		ok, state, err := probe()
		if err == nil && ok {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = nil
			lastState = state
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if remaining > e2ePollInterval {
			remaining = e2ePollInterval
		}
		time.Sleep(remaining)
	}
	if lastErr != nil {
		return fmt.Errorf("%s超时，最后错误: %w", description, lastErr)
	}
	lastState = strings.TrimSpace(lastState)
	if lastState == "" {
		return fmt.Errorf("%s超时", description)
	}
	return fmt.Errorf("%s超时，最后状态: %s", description, lastState)
}

// expectPageState 使用 page、description、script、arg 和 timeout 参数等待浏览器脚本返回空状态。
func expectPageState(page playwright.Page, description string, script string, arg any, timeout time.Duration) error {
	return waitForCondition(description, timeout, func() (bool, string, error) {
		value, err := page.Evaluate(script, arg)
		if err != nil {
			return false, "", err
		}
		state := fmt.Sprint(value)
		return state == "", state, nil
	})
}

// expectLocatorState 使用 locator、description、script、arg 和 timeout 参数等待元素脚本返回空状态。
func expectLocatorState(locator playwright.Locator, description string, script string, arg any, timeout time.Duration) error {
	return waitForCondition(description, timeout, func() (bool, string, error) {
		value, err := locator.First().Evaluate(script, arg)
		if err != nil {
			return false, "", err
		}
		state := fmt.Sprint(value)
		return state == "", state, nil
	})
}
