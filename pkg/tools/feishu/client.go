//go:build amd64 || arm64 || riscv64 || mips64 || ppc64

// Package feishu 提供飞书 API 工具集，让 Agent 能够主动调用飞书各类能力。
// 仅支持 64 位架构（与飞书 SDK 限制一致）。
package feishu

import (
	lark "github.com/larksuite/oapi-sdk-go/v3"
)

// Client 封装飞书 lark.Client，作为所有飞书工具的共享依赖。
type Client struct {
	lark *lark.Client
}

// NewClient 使用 AppID 和 AppSecret 创建飞书客户端。
func NewClient(appID, appSecret string) *Client {
	return &Client{
		lark: lark.NewClient(appID, appSecret),
	}
}
