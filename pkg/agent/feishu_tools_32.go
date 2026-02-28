//go:build !amd64 && !arm64 && !riscv64 && !mips64 && !ppc64

package agent

import (
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// registerFeishuTools 在 32 位架构上为空操作（飞书 SDK 不支持 32 位）。
func registerFeishuTools(_ *tools.ToolRegistry, _ *config.Config, _ string) {}
