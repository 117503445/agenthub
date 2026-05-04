package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/playwright-community/playwright-go"
)

// agentProfileEnvEntry 表示 E2E 断言使用的环境变量项。
type agentProfileEnvEntry struct {
	Name  string `json:"name"`  // Name 表示环境变量名。
	Value string `json:"value"` // Value 表示环境变量值。
	Unset bool   `json:"unset"` // Unset 表示是否删除后端同名环境变量。
}

// agentProfileModel 表示 E2E 断言使用的 profile 模型项。
type agentProfileModel struct {
	ID      string `json:"id"`      // ID 表示模型标识。
	Label   string `json:"label"`   // Label 表示模型展示值，与 ID 保持一致。
	Default bool   `json:"default"` // Default 表示是否默认模型。
}

// agentProfileSnapshotItem 表示 E2E 断言使用的 agent profile。
type agentProfileSnapshotItem struct {
	ID      string                 `json:"id"`      // ID 表示 profile 标识。
	Label   string                 `json:"label"`   // Label 表示 profile 展示名称。
	Type    string                 `json:"type"`    // Type 表示 profile 类型。
	Command string                 `json:"command"` // Command 表示启动命令。
	Args    []string               `json:"args"`    // Args 表示固定参数。
	Env     []agentProfileEnvEntry `json:"env"`     // Env 表示 profile 环境变量配置。
	Models  []agentProfileModel    `json:"models"`  // Models 表示可切换模型。
	Builtin bool                   `json:"builtin"` // Builtin 表示是否内置 profile。
}

// agentProfileSnapshotPayload 表示 E2E 读取的状态快照。
type agentProfileSnapshotPayload struct {
	AgentProfiles []agentProfileSnapshotItem `json:"agentProfiles"` // AgentProfiles 表示 profile 列表。
	BackendEnv    []agentProfileEnvEntry     `json:"backendEnv"`    // BackendEnv 表示后端启动环境变量。
}

// agentProfilesChangedPayload 表示 profile 变更事件。
type agentProfilesChangedPayload struct {
	AgentProfiles []agentProfileSnapshotItem `json:"agentProfiles"` // AgentProfiles 表示变更后的 profile 列表。
}

// agentProfileEffectiveEnvPayload 表示 effective env 响应。
type agentProfileEffectiveEnvPayload struct {
	ID  string                 `json:"id"`  // ID 表示 profile 标识。
	Env []agentProfileEnvEntry `json:"env"` // Env 表示叠加后的完整环境变量。
}

// runAgentProfileCase 使用 ctx 参数运行 Agent Profile 管理 E2E 用例。
func runAgentProfileCase(ctx E2EContext) (success bool) {
	events := make([]reportEvent, 0)
	defer func() {
		writeAgentProfileReport(ctx.OutputDir, success, events)
	}()

	fail := func(err error) bool {
		ctx.Logger.Errorf("Agent Profile 管理 E2E 失败: %v", err)
		events = append(events, reportStep(fmt.Sprintf("用例失败: %v", err)))
		return false
	}

	conn, err := dialPersistenceWS(ctx.BaseURL)
	if err != nil {
		return fail(err)
	}
	defer conn.CloseNow()

	snapshotMessage, err := readPersistenceMessage(conn, "state.snapshot", 5*time.Second)
	if err != nil {
		return fail(err)
	}
	var snapshot agentProfileSnapshotPayload
	if err := json.Unmarshal(snapshotMessage.Payload, &snapshot); err != nil {
		return fail(err)
	}
	if err := assertInitialAgentProfiles(snapshot); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("首次初始化数据目录时，状态快照返回真实和 Mock 内置 Profile，并展示后端启动环境变量。"))

	if err := sendAgentProfileMessage(conn, "agent.profile.delete", map[string]string{"id": "mock-codex"}); err != nil {
		return fail(err)
	}
	if _, err := readPersistenceMessage(conn, "agent.profiles.changed", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := sendAgentProfileMessage(conn, "agent.profile.add_builtin", map[string]string{"kind": "mock_codex"}); err != nil {
		return fail(err)
	}
	changedMessage, err := readPersistenceMessage(conn, "agent.profiles.changed", 5*time.Second)
	if err != nil {
		return fail(err)
	}
	profiles, err := decodeAgentProfilesChanged(changedMessage.Payload)
	if err != nil {
		return fail(err)
	}
	if !agentProfileExists(profiles, "mock-codex") {
		return fail(fmt.Errorf("重新添加内置 mock-codex 后未出现在 profile 列表中: %#v", profiles))
	}
	events = append(events, reportStep("删除过的内置 Profile 可以通过新增内置 Profile 再次添加。"))

	profile := agentProfileByID(profiles, "mock-codex")
	profile.Env = append(profile.Env,
		agentProfileEnvEntry{Name: "AGENTHUB_PROFILE_E2E_SECRET", Value: "profile-secret-value"},
		agentProfileEnvEntry{Name: "AGENTHUB_PROFILE_E2E_UNSET", Unset: true},
	)
	if err := sendAgentProfileMessage(conn, "agent.profile.update", profile); err != nil {
		return fail(err)
	}
	if _, err := readPersistenceMessage(conn, "agent.profiles.changed", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := sendAgentProfileMessage(conn, "agent.profile.effective_env.get", map[string]string{"id": "mock-codex"}); err != nil {
		return fail(err)
	}
	envMessage, err := readPersistenceMessage(conn, "agent.profile.effective_env", 5*time.Second)
	if err != nil {
		return fail(err)
	}
	if err := assertEffectiveEnv(envMessage.Payload); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("Effective Env 使用后端启动环境变量叠加 Profile 环境变量，支持覆盖和 unset，并返回完整密钥值。"))

	customProfile := agentProfileSnapshotItem{
		ID:      "custom-codex-e2e",
		Label:   "Custom Codex E2E",
		Type:    "codex",
		Command: "codex",
		Args:    []string{"--skip-git-repo-check"},
		Env:     []agentProfileEnvEntry{{Name: "CUSTOM_PROFILE_ENV", Value: "custom-value"}},
		Models:  []agentProfileModel{{ID: "custom-model", Label: "custom-model", Default: true}},
	}
	if err := sendAgentProfileMessage(conn, "agent.profile.create", customProfile); err != nil {
		return fail(err)
	}
	if _, err := readPersistenceMessage(conn, "agent.profiles.changed", 5*time.Second); err != nil {
		return fail(err)
	}
	if err := sendAgentProfileMessage(conn, "agent.profile.model.add", map[string]string{"profileId": "custom-codex-e2e", "id": "custom-model-2"}); err != nil {
		return fail(err)
	}
	if err := assertProfileChangedContainsModel(conn, "custom-codex-e2e", "custom-model-2"); err != nil {
		return fail(err)
	}
	if err := sendAgentProfileMessage(conn, "agent.profile.model.update", map[string]any{"profileId": "custom-codex-e2e", "id": "custom-model-2", "default": true}); err != nil {
		return fail(err)
	}
	if err := assertProfileChangedDefaultModel(conn, "custom-codex-e2e", "custom-model-2"); err != nil {
		return fail(err)
	}
	if err := sendAgentProfileMessage(conn, "agent.profile.model.delete", map[string]string{"profileId": "custom-codex-e2e", "id": "custom-model-2"}); err != nil {
		return fail(err)
	}
	if err := assertProfileChangedMissingModel(conn, "custom-codex-e2e", "custom-model-2"); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("Profile 和模型项支持增删改查。"))

	if err := assertAgentProfileSettingsUI(ctx); err != nil {
		return fail(err)
	}
	events = append(events, reportStep("设置页展示 Profile 列表、后端启动环境变量和完整 Effective Env。"))
	return true
}

// sendAgentProfileMessage 使用 conn、messageType 和 payload 参数发送 profile 管理消息。
func sendAgentProfileMessage(conn *websocket.Conn, messageType string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return wsjson.Write(ctx, conn, persistenceClientMessage{Type: messageType, Payload: data})
}

// assertInitialAgentProfiles 使用 snapshot 参数断言内置 profile 和后端环境变量。
func assertInitialAgentProfiles(snapshot agentProfileSnapshotPayload) error {
	for _, id := range []string{"claude-code", "codex", "mock-claude-code", "mock-codex"} {
		if !agentProfileExists(snapshot.AgentProfiles, id) {
			return fmt.Errorf("初始 profile 缺少 %s: %#v", id, snapshot.AgentProfiles)
		}
	}
	if !agentEnvValueExists(snapshot.BackendEnv, "AGENTHUB_PROFILE_E2E_SECRET", "backend-secret-value") {
		return fmt.Errorf("后端启动环境变量没有完整返回测试密钥: %#v", snapshot.BackendEnv)
	}
	for _, profile := range snapshot.AgentProfiles {
		if profile.ID == "claude-code" && profile.Type != "claude_code" {
			return fmt.Errorf("Claude Code profile 类型不正确: %#v", profile)
		}
		if profile.ID == "codex" && profile.Type != "codex" {
			return fmt.Errorf("Codex profile 类型不正确: %#v", profile)
		}
	}
	return nil
}

// decodeAgentProfilesChanged 使用 data 参数解析 profile 变更事件。
func decodeAgentProfilesChanged(data json.RawMessage) ([]agentProfileSnapshotItem, error) {
	var payload agentProfilesChangedPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload.AgentProfiles, nil
}

// assertEffectiveEnv 使用 data 参数断言环境变量覆盖和 unset。
func assertEffectiveEnv(data json.RawMessage) error {
	var payload agentProfileEffectiveEnvPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if !agentEnvValueExists(payload.Env, "AGENTHUB_PROFILE_E2E_SECRET", "profile-secret-value") {
		return fmt.Errorf("Effective Env 未使用 Profile 环境变量覆盖后端变量: %#v", payload.Env)
	}
	if agentEnvNameExists(payload.Env, "AGENTHUB_PROFILE_E2E_UNSET") {
		return fmt.Errorf("Effective Env 未 unset 后端变量: %#v", payload.Env)
	}
	return nil
}

// assertProfileChangedContainsModel 使用 conn、profileID 和 modelID 参数断言模型存在且展示值等于标识。
func assertProfileChangedContainsModel(conn *websocket.Conn, profileID string, modelID string) error {
	profiles, err := readAgentProfilesChanged(conn)
	if err != nil {
		return err
	}
	for _, model := range agentProfileByID(profiles, profileID).Models {
		if model.ID == modelID && model.Label == modelID {
			return nil
		}
	}
	return fmt.Errorf("profile %s 缺少模型 %s 或展示值不等于标识: %#v", profileID, modelID, profiles)
}

// assertProfileChangedDefaultModel 使用 conn、profileID 和 modelID 参数断言默认模型。
func assertProfileChangedDefaultModel(conn *websocket.Conn, profileID string, modelID string) error {
	profiles, err := readAgentProfilesChanged(conn)
	if err != nil {
		return err
	}
	for _, model := range agentProfileByID(profiles, profileID).Models {
		if model.ID == modelID && model.Default {
			return nil
		}
	}
	return fmt.Errorf("profile %s 默认模型未更新为 %s: %#v", profileID, modelID, profiles)
}

// assertProfileChangedMissingModel 使用 conn、profileID 和 modelID 参数断言模型已删除。
func assertProfileChangedMissingModel(conn *websocket.Conn, profileID string, modelID string) error {
	profiles, err := readAgentProfilesChanged(conn)
	if err != nil {
		return err
	}
	if agentProfileModelExists(agentProfileByID(profiles, profileID).Models, modelID) {
		return fmt.Errorf("profile %s 模型 %s 未删除: %#v", profileID, modelID, profiles)
	}
	return nil
}

// readAgentProfilesChanged 使用 conn 参数读取 profile 变更事件。
func readAgentProfilesChanged(conn *websocket.Conn) ([]agentProfileSnapshotItem, error) {
	message, err := readPersistenceMessage(conn, "agent.profiles.changed", 5*time.Second)
	if err != nil {
		return nil, err
	}
	return decodeAgentProfilesChanged(message.Payload)
}

// assertAgentProfileSettingsUI 使用 ctx 参数验证设置页 profile 信息展示。
func assertAgentProfileSettingsUI(ctx E2EContext) error {
	session, err := newBrowserSession(1360, 860)
	if err != nil {
		return err
	}
	defer session.Close()
	page := session.page
	if err := gotoPage(page, ctx.BaseURL); err != nil {
		return err
	}
	if err := expectTestIDText(page, "connection-state", "已连接", 10*time.Second); err != nil {
		return err
	}
	if err := clickTestID(page, "agent-settings-button"); err != nil {
		return err
	}
	if err := expectTestIDCount(page, "agent-model-label-input", 0, 2*time.Second); err != nil {
		return err
	}
	if err := expectTestIDText(page, "agent-profile-list", "Custom Codex E2E", 10*time.Second); err != nil {
		return err
	}
	if err := page.Locator(`[data-testid="agent-profile-list"] button`, playwright.PageLocatorOptions{HasText: "Mock Codex"}).Click(); err != nil {
		return err
	}
	if err := expectTestIDText(page, "agent-settings-backend-env", "AGENTHUB_PROFILE_E2E_SECRET", 10*time.Second); err != nil {
		return err
	}
	if err := expectTestIDText(page, "agent-settings-backend-env", "backend-secret-value", 10*time.Second); err != nil {
		return err
	}
	if err := expectTestIDText(page, "agent-profile-effective-env", "profile-secret-value", 10*time.Second); err != nil {
		return err
	}
	if err := page.Locator(`[data-testid="agent-model-delete-button"][data-model-id="mock-codex-fast"]`).Click(); err != nil {
		return err
	}
	if err := expectTestIDNotText(page, "agent-settings-model-list", "mock-codex-fast", 5*time.Second); err != nil {
		return err
	}
	screenshot(page, filepath.Join(ctx.ScreenshotsDir, "01-agent-profile-settings.png"), true)
	return nil
}

// agentProfileExists 使用 profiles 和 id 参数判断 profile 是否存在。
func agentProfileExists(profiles []agentProfileSnapshotItem, id string) bool {
	return agentProfileByID(profiles, id).ID != ""
}

// agentProfileByID 使用 profiles 和 id 参数查找 profile。
func agentProfileByID(profiles []agentProfileSnapshotItem, id string) agentProfileSnapshotItem {
	for _, profile := range profiles {
		if profile.ID == id {
			return profile
		}
	}
	return agentProfileSnapshotItem{}
}

// agentProfileModelExists 使用 models 和 id 参数判断模型是否存在。
func agentProfileModelExists(models []agentProfileModel, id string) bool {
	for _, model := range models {
		if model.ID == id {
			return true
		}
	}
	return false
}

// agentEnvValueExists 使用 env、name 和 value 参数判断环境变量值是否存在。
func agentEnvValueExists(env []agentProfileEnvEntry, name string, value string) bool {
	for _, item := range env {
		if item.Name == name && item.Value == value {
			return true
		}
	}
	return false
}

// agentEnvNameExists 使用 env 和 name 参数判断环境变量名是否存在。
func agentEnvNameExists(env []agentProfileEnvEntry, name string) bool {
	for _, item := range env {
		if item.Name == name {
			return true
		}
	}
	return false
}

// writeAgentProfileReport 使用 outputDir、success 和 events 参数写入 Agent Profile 管理报告。
func writeAgentProfileReport(outputDir string, success bool, events []reportEvent) {
	writeE2EReport(outputDir, "Agent Profile 管理 E2E 测试报告", success, events)
}
