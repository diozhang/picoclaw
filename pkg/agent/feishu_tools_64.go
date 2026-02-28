//go:build amd64 || arm64 || riscv64 || mips64 || ppc64

package agent

import (
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/tools"
	feishutools "github.com/sipeed/picoclaw/pkg/tools/feishu"
)

// registerFeishuTools 在 64 位架构上注册飞书 API 工具集。
// 仅当飞书通道已启用且 feishu.tools.enabled=true 时才注册。
func registerFeishuTools(registry *tools.ToolRegistry, cfg *config.Config, workspace string) {
	feishuCfg := cfg.Channels.Feishu
	if !feishuCfg.Enabled || !feishuCfg.Tools.Enabled {
		return
	}
	if feishuCfg.AppID == "" || feishuCfg.AppSecret == "" {
		return
	}
	feishutools.RegisterFeishuTools(registry, feishuCfg.AppID, feishuCfg.AppSecret, workspace)
}
