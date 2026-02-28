//go:build amd64 || arm64 || riscv64 || mips64 || ppc64

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/sipeed/picoclaw/pkg/tools"
)

// ============================================================
// feishu_send_message
// ============================================================

// SendMessageTool 向指定会话发送消息，支持文本、富文本（Markdown 自动转换）和卡片消息。
type SendMessageTool struct {
	client *Client
}

func newSendMessageTool(c *Client) *SendMessageTool {
	return &SendMessageTool{client: c}
}

func (t *SendMessageTool) Name() string { return "feishu_send_message" }

func (t *SendMessageTool) Description() string {
	return "向飞书指定会话发送消息。支持三种消息类型：" +
		"text（纯文本）、" +
		"markdown（富文本，自动将 Markdown 转换为飞书 post 格式，支持标题/加粗/代码块/链接）、" +
		"card（消息卡片，需传入飞书卡片 JSON 字符串）。" +
		"receive_id_type 指定接收者类型：chat_id（群聊，默认）、open_id、user_id。"
}

func (t *SendMessageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"receive_id": map[string]any{
				"type":        "string",
				"description": "接收者 ID，与 receive_id_type 对应",
			},
			"receive_id_type": map[string]any{
				"type":        "string",
				"description": "接收者 ID 类型：chat_id（默认）、open_id、user_id、union_id、email",
				"enum":        []string{"chat_id", "open_id", "user_id", "union_id", "email"},
			},
			"msg_type": map[string]any{
				"type":        "string",
				"description": "消息类型：text（纯文本）、markdown（自动转为富文本）、card（卡片 JSON）",
				"enum":        []string{"text", "markdown", "card"},
			},
			"content": map[string]any{
				"type":        "string",
				"description": "消息内容。text 类型传纯文本；markdown 类型传 Markdown 字符串；card 类型传飞书卡片 JSON 字符串",
			},
		},
		"required": []string{"receive_id", "msg_type", "content"},
	}
}

func (t *SendMessageTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	receiveID, _ := args["receive_id"].(string)
	receiveIDType, _ := args["receive_id_type"].(string)
	msgType, _ := args["msg_type"].(string)
	content, _ := args["content"].(string)

	if receiveID == "" {
		return tools.ErrorResult("receive_id 不能为空")
	}
	if content == "" {
		return tools.ErrorResult("content 不能为空")
	}
	if receiveIDType == "" {
		receiveIDType = larkim.ReceiveIdTypeChatId
	}

	larkMsgType, payload, err := buildMessagePayload(msgType, content)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("构建消息内容失败: %v", err))
	}

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(receiveIDType).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(receiveID).
			MsgType(larkMsgType).
			Content(payload).
			Uuid(fmt.Sprintf("picoclaw-%d", time.Now().UnixNano())).
			Build()).
		Build()

	resp, err := t.client.lark.Im.V1.Message.Create(ctx, req)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("发送消息失败: %v", err))
	}
	if !resp.Success() {
		return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", resp.Code, resp.Msg))
	}

	msgID := ""
	if resp.Data != nil && resp.Data.MessageId != nil {
		msgID = *resp.Data.MessageId
	}
	return tools.SilentResult(fmt.Sprintf("消息发送成功，message_id=%s", msgID))
}

// ============================================================
// feishu_reply_message
// ============================================================

// ReplyMessageTool 回复指定消息。
type ReplyMessageTool struct {
	client *Client
}

func newReplyMessageTool(c *Client) *ReplyMessageTool {
	return &ReplyMessageTool{client: c}
}

func (t *ReplyMessageTool) Name() string { return "feishu_reply_message" }

func (t *ReplyMessageTool) Description() string {
	return "回复飞书中的指定消息。支持 text、markdown、card 三种消息类型。" +
		"需要提供原消息的 message_id（可从消息事件的 metadata 中获取）。"
}

func (t *ReplyMessageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message_id": map[string]any{
				"type":        "string",
				"description": "被回复消息的 ID，格式如 om_xxx",
			},
			"msg_type": map[string]any{
				"type":        "string",
				"description": "消息类型：text、markdown、card",
				"enum":        []string{"text", "markdown", "card"},
			},
			"content": map[string]any{
				"type":        "string",
				"description": "消息内容",
			},
		},
		"required": []string{"message_id", "msg_type", "content"},
	}
}

func (t *ReplyMessageTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	messageID, _ := args["message_id"].(string)
	msgType, _ := args["msg_type"].(string)
	content, _ := args["content"].(string)

	if messageID == "" {
		return tools.ErrorResult("message_id 不能为空")
	}
	if content == "" {
		return tools.ErrorResult("content 不能为空")
	}

	larkMsgType, payload, err := buildMessagePayload(msgType, content)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("构建消息内容失败: %v", err))
	}

	req := larkim.NewReplyMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewReplyMessageReqBodyBuilder().
			MsgType(larkMsgType).
			Content(payload).
			Uuid(fmt.Sprintf("picoclaw-reply-%d", time.Now().UnixNano())).
			Build()).
		Build()

	resp, err := t.client.lark.Im.V1.Message.Reply(ctx, req)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("回复消息失败: %v", err))
	}
	if !resp.Success() {
		return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", resp.Code, resp.Msg))
	}

	replyID := ""
	if resp.Data != nil && resp.Data.MessageId != nil {
		replyID = *resp.Data.MessageId
	}
	return tools.SilentResult(fmt.Sprintf("回复成功，message_id=%s", replyID))
}

// ============================================================
// feishu_get_message
// ============================================================

// GetMessageTool 获取指定消息的详情。
type GetMessageTool struct {
	client *Client
}

func newGetMessageTool(c *Client) *GetMessageTool {
	return &GetMessageTool{client: c}
}

func (t *GetMessageTool) Name() string { return "feishu_get_message" }

func (t *GetMessageTool) Description() string {
	return "获取飞书指定消息的详情，包括消息类型、内容、发送者、发送时间等信息。"
}

func (t *GetMessageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message_id": map[string]any{
				"type":        "string",
				"description": "消息 ID，格式如 om_xxx",
			},
		},
		"required": []string{"message_id"},
	}
}

func (t *GetMessageTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	messageID, _ := args["message_id"].(string)
	if messageID == "" {
		return tools.ErrorResult("message_id 不能为空")
	}

	req := larkim.NewGetMessageReqBuilder().
		MessageId(messageID).
		Build()

	resp, err := t.client.lark.Im.V1.Message.Get(ctx, req)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("获取消息失败: %v", err))
	}
	if !resp.Success() {
		return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", resp.Code, resp.Msg))
	}

	if resp.Data == nil || len(resp.Data.Items) == 0 {
		return tools.SilentResult("未找到该消息")
	}

	msg := resp.Data.Items[0]
	result := map[string]any{
		"message_id":   strVal(msg.MessageId),
		"msg_type":     strVal(msg.MsgType),
		"content":      strVal(msg.Body.Content),
		"sender_id":    strVal(msg.Sender.Id),
		"sender_type":  strVal(msg.Sender.SenderType),
		"create_time":  strVal(msg.CreateTime),
		"update_time":  strVal(msg.UpdateTime),
		"chat_id":      strVal(msg.ChatId),
		"thread_id":    strVal(msg.ThreadId),
		"parent_id":    strVal(msg.ParentId),
		"deleted":      boolVal(msg.Deleted),
		"updated":      boolVal(msg.Updated),
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return tools.SilentResult(string(out))
}

// ============================================================
// feishu_list_messages
// ============================================================

// ListMessagesTool 获取会话的历史消息列表。
type ListMessagesTool struct {
	client *Client
}

func newListMessagesTool(c *Client) *ListMessagesTool {
	return &ListMessagesTool{client: c}
}

func (t *ListMessagesTool) Name() string { return "feishu_list_messages" }

func (t *ListMessagesTool) Description() string {
	return "获取飞书会话（群聊或私聊）的历史消息列表，支持按时间范围过滤，按时间升序或降序排列。"
}

func (t *ListMessagesTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"container_id": map[string]any{
				"type":        "string",
				"description": "会话 ID（chat_id）",
			},
			"start_time": map[string]any{
				"type":        "string",
				"description": "查询起始时间，Unix 时间戳（秒），可选",
			},
			"end_time": map[string]any{
				"type":        "string",
				"description": "查询结束时间，Unix 时间戳（秒），可选",
			},
			"sort_type": map[string]any{
				"type":        "string",
				"description": "排序方式：ByCreateTimeAsc（时间升序，默认）、ByCreateTimeDesc（时间降序）",
				"enum":        []string{"ByCreateTimeAsc", "ByCreateTimeDesc"},
			},
			"page_size": map[string]any{
				"type":        "integer",
				"description": "每页消息数量，最大 50，默认 20",
			},
		},
		"required": []string{"container_id"},
	}
}

func (t *ListMessagesTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	containerID, _ := args["container_id"].(string)
	if containerID == "" {
		return tools.ErrorResult("container_id 不能为空")
	}

	startTime, _ := args["start_time"].(string)
	endTime, _ := args["end_time"].(string)
	sortType, _ := args["sort_type"].(string)
	if sortType == "" {
		sortType = larkim.SortTypeListMessageByCreateTimeAsc
	}

	pageSize := 20
	if v, ok := args["page_size"].(float64); ok && v > 0 {
		pageSize = int(v)
		if pageSize > 50 {
			pageSize = 50
		}
	}

	builder := larkim.NewListMessageReqBuilder().
		ContainerIdType("chat").
		ContainerId(containerID).
		SortType(sortType).
		PageSize(pageSize)

	if startTime != "" {
		builder = builder.StartTime(startTime)
	}
	if endTime != "" {
		builder = builder.EndTime(endTime)
	}

	resp, err := t.client.lark.Im.V1.Message.List(ctx, builder.Build())
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("获取消息列表失败: %v", err))
	}
	if !resp.Success() {
		return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", resp.Code, resp.Msg))
	}

	if resp.Data == nil || len(resp.Data.Items) == 0 {
		return tools.SilentResult("该会话暂无消息")
	}

	type msgSummary struct {
		MessageID  string `json:"message_id"`
		MsgType    string `json:"msg_type"`
		Content    string `json:"content"`
		SenderID   string `json:"sender_id"`
		CreateTime string `json:"create_time"`
	}

	items := make([]msgSummary, 0, len(resp.Data.Items))
	for _, m := range resp.Data.Items {
		content := ""
		if m.Body != nil {
			content = strVal(m.Body.Content)
		}
		items = append(items, msgSummary{
			MessageID:  strVal(m.MessageId),
			MsgType:    strVal(m.MsgType),
			Content:    content,
			SenderID:   strVal(m.Sender.Id),
			CreateTime: strVal(m.CreateTime),
		})
	}

	out, _ := json.MarshalIndent(map[string]any{
		"total":    len(items),
		"messages": items,
	}, "", "  ")
	return tools.SilentResult(string(out))
}

// ============================================================
// feishu_get_message_resource
// ============================================================

// GetMessageResourceTool 下载消息中的图片、文件、音频或视频，保存到工作区。
type GetMessageResourceTool struct {
	client    *Client
	workspace string
}

func newGetMessageResourceTool(c *Client, workspace string) *GetMessageResourceTool {
	return &GetMessageResourceTool{client: c, workspace: workspace}
}

func (t *GetMessageResourceTool) Name() string { return "feishu_get_message_resource" }

func (t *GetMessageResourceTool) Description() string {
	return "下载飞书消息中的媒体资源（图片、文件、音频、视频）并保存到本地工作区。" +
		"注意：仅能下载当前机器人自己上传的资源文件；用户发送的文件无法下载。" +
		"type 参数：image 对应图片，file 对应文件/音频/视频。" +
		"成功后返回保存的本地文件路径。"
}

func (t *GetMessageResourceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message_id": map[string]any{
				"type":        "string",
				"description": "资源所在消息的 ID",
			},
			"file_key": map[string]any{
				"type":        "string",
				"description": "资源的 key（image_key 或 file_key），从消息内容中获取",
			},
			"type": map[string]any{
				"type":        "string",
				"description": "资源类型：image（图片）或 file（文件/音频/视频）",
				"enum":        []string{"image", "file"},
			},
			"filename": map[string]any{
				"type":        "string",
				"description": "保存的文件名（可选），不填则自动生成",
			},
		},
		"required": []string{"message_id", "file_key", "type"},
	}
}

func (t *GetMessageResourceTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	messageID, _ := args["message_id"].(string)
	fileKey, _ := args["file_key"].(string)
	resType, _ := args["type"].(string)
	filename, _ := args["filename"].(string)

	if messageID == "" {
		return tools.ErrorResult("message_id 不能为空")
	}
	if fileKey == "" {
		return tools.ErrorResult("file_key 不能为空")
	}
	if resType != "image" && resType != "file" {
		return tools.ErrorResult("type 必须为 image 或 file")
	}

	req := larkim.NewGetMessageResourceReqBuilder().
		MessageId(messageID).
		FileKey(fileKey).
		Type(resType).
		Build()

	resp, err := t.client.lark.Im.V1.MessageResource.Get(ctx, req)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("下载资源失败: %v", err))
	}
	if !resp.Success() {
		return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", resp.Code, resp.Msg))
	}

	saveName := filename
	if saveName == "" {
		if resp.FileName != "" {
			saveName = resp.FileName
		} else {
			saveName = fmt.Sprintf("feishu_%s_%d", resType, time.Now().UnixNano())
		}
	}

	savePath := filepath.Join(t.workspace, saveName)
	if err := saveStream(resp.File, savePath); err != nil {
		return tools.ErrorResult(fmt.Sprintf("保存文件失败: %v", err))
	}

	return tools.SilentResult(fmt.Sprintf("资源已保存到: %s", savePath))
}

// ============================================================
// 内部工具函数
// ============================================================

// buildMessagePayload 将 msg_type 和 content 转换为飞书 API 所需的消息类型和 payload JSON。
func buildMessagePayload(msgType, content string) (larkMsgType string, payload string, err error) {
	switch msgType {
	case "text":
		larkMsgType = larkim.MsgTypeText
		b, e := json.Marshal(map[string]string{"text": content})
		if e != nil {
			return "", "", e
		}
		payload = string(b)

	case "markdown":
		// 将 Markdown 转换为飞书 post 富文本格式
		larkMsgType = larkim.MsgTypePost
		postContent := convertMarkdownToPost(content)
		b, e := json.Marshal(postContent)
		if e != nil {
			return "", "", e
		}
		payload = string(b)

	case "card":
		// 卡片消息直接使用用户传入的 JSON
		larkMsgType = larkim.MsgTypeInteractive
		payload = content

	default:
		// 未知类型降级为纯文本
		larkMsgType = larkim.MsgTypeText
		b, e := json.Marshal(map[string]string{"text": content})
		if e != nil {
			return "", "", e
		}
		payload = string(b)
	}
	return larkMsgType, payload, nil
}

// convertMarkdownToPost 将 Markdown 字符串转换为飞书 post 富文本 JSON 结构。
// 使用飞书原生支持的 md 标签，直接传入 Markdown 字符串，飞书会自动渲染。
// 支持：@用户、超链接、有序/无序列表、代码块、引用、分割线、加粗、斜体、下划线、删除线。
func convertMarkdownToPost(md string) map[string]any {
	return map[string]any{
		"zh_cn": map[string]any{
			"title": "",
			"content": [][]map[string]any{
				{
					{"tag": "md", "text": md},
				},
			},
		},
	}
}

// saveStream 将 io.Reader 的内容保存到指定路径。
func saveStream(r io.Reader, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	return nil
}

// strVal 安全解引用 *string 指针。
func strVal(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// boolVal 安全解引用 *bool 指针。
func boolVal(v *bool) bool {
	if v == nil {
		return false
	}
	return *v
}
