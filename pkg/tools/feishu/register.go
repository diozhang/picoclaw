//go:build amd64 || arm64 || riscv64 || mips64 || ppc64

// Package feishu 提供飞书 API 工具集，让 Agent 能够主动调用飞书各类能力。
// 工具清单（14 个）：
//
// 消息类（6个）：
//   - feishu_send_message       向指定会话发送文本/富文本/卡片消息
//   - feishu_reply_message      回复指定消息
//   - feishu_get_message        获取消息详情
//   - feishu_list_messages      获取会话历史消息列表
//   - feishu_get_message_resource 下载消息中的图片/文件/音视频
//   - feishu_add_reaction       给消息添加表情回复（+1、OK、爱心等）
//
// 通讯录类（4个）：
//   - feishu_get_user           获取用户详情
//   - feishu_search_user        按邮箱/手机号搜索用户
//   - feishu_get_department     获取部门信息
//   - feishu_list_department_users 获取部门成员列表
//
// 日历类（2个）：
//   - feishu_list_events        查询日历事件列表
//   - feishu_create_event       创建日历事件
//
// 云文档类（2个）：
//   - feishu_get_doc_content    获取文档纯文本内容及媒体资源列表
//   - feishu_download_doc_media 下载文档中的图片/附件
package feishu

import (
	"github.com/sipeed/picoclaw/pkg/tools"
)

// RegisterFeishuTools 将所有飞书工具注册到指定的 ToolRegistry。
// appID、appSecret 用于初始化飞书客户端；workspace 为媒体文件的本地保存目录。
func RegisterFeishuTools(registry *tools.ToolRegistry, appID, appSecret, workspace string) {
	client := NewClient(appID, appSecret)

	// 消息类
	registry.Register(newSendMessageTool(client))
	registry.Register(newReplyMessageTool(client))
	registry.Register(newGetMessageTool(client))
	registry.Register(newListMessagesTool(client))
	registry.Register(newGetMessageResourceTool(client, workspace))
	registry.Register(newAddReactionTool(client))

	// 通讯录类
	registry.Register(newGetUserTool(client))
	registry.Register(newSearchUserTool(client))
	registry.Register(newGetDepartmentTool(client))
	registry.Register(newListDepartmentUsersTool(client))

	// 日历类
	registry.Register(newListEventsTool(client))
	registry.Register(newCreateEventTool(client))

	// 云文档类
	registry.Register(newGetDocContentTool(client))
	registry.Register(newDownloadDocMediaTool(client, workspace))
}
