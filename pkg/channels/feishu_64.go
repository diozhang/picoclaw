//go:build amd64 || arm64 || riscv64 || mips64 || ppc64

package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkdispatcher "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/utils"
)

type FeishuChannel struct {
	*BaseChannel
	config   config.FeishuConfig
	client   *lark.Client
	wsClient *larkws.Client

	mu     sync.Mutex
	cancel context.CancelFunc
}

func NewFeishuChannel(cfg config.FeishuConfig, bus *bus.MessageBus) (*FeishuChannel, error) {
	base := NewBaseChannel("feishu", cfg, bus, cfg.AllowFrom)

	return &FeishuChannel{
		BaseChannel: base,
		config:      cfg,
		client:      lark.NewClient(cfg.AppID, cfg.AppSecret),
	}, nil
}

func (c *FeishuChannel) Start(ctx context.Context) error {
	if c.config.AppID == "" || c.config.AppSecret == "" {
		return fmt.Errorf("feishu app_id or app_secret is empty")
	}

	dispatcher := larkdispatcher.NewEventDispatcher(c.config.VerificationToken, c.config.EncryptKey).
		OnP2MessageReceiveV1(c.handleMessageReceive)

	runCtx, cancel := context.WithCancel(ctx)

	c.mu.Lock()
	c.cancel = cancel
	c.wsClient = larkws.NewClient(
		c.config.AppID,
		c.config.AppSecret,
		larkws.WithEventHandler(dispatcher),
	)
	wsClient := c.wsClient
	c.mu.Unlock()

	c.setRunning(true)
	logger.InfoC("feishu", "Feishu channel started (websocket mode)")

	go func() {
		if err := wsClient.Start(runCtx); err != nil {
			logger.ErrorCF("feishu", "Feishu websocket stopped with error", map[string]any{
				"error": err.Error(),
			})
		}
	}()

	return nil
}

func (c *FeishuChannel) Stop(ctx context.Context) error {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.wsClient = nil
	c.mu.Unlock()

	c.setRunning(false)
	logger.InfoC("feishu", "Feishu channel stopped")
	return nil
}

func (c *FeishuChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return fmt.Errorf("feishu channel not running")
	}

	if msg.ChatID == "" {
		return fmt.Errorf("chat ID is empty")
	}

	msgType, payload, err := buildFeishuPayload(msg.Content)
	if err != nil {
		return fmt.Errorf("failed to marshal feishu content: %w", err)
	}

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(msg.ChatID).
			MsgType(msgType).
			Content(payload).
			Uuid(fmt.Sprintf("picoclaw-%d", time.Now().UnixNano())).
			Build()).
		Build()

	resp, err := c.client.Im.V1.Message.Create(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send feishu message: %w", err)
	}

	if !resp.Success() {
		return fmt.Errorf("feishu api error: code=%d msg=%s", resp.Code, resp.Msg)
	}

	logger.DebugCF("feishu", "Feishu message sent", map[string]any{
		"chat_id":  msg.ChatID,
		"msg_type": msgType,
	})

	return nil
}

// buildFeishuPayload 根据消息内容自动选择消息类型：
// 含 Markdown 语法时使用 post 富文本的 md 标签（飞书原生支持 Markdown 渲染），否则发纯文本。
func buildFeishuPayload(content string) (msgType string, payload string, err error) {
	if containsMarkdown(content) {
		// 使用飞书 post 富文本的 md 标签，直接传入 Markdown 字符串，飞书会自动渲染
		// 参考文档：md 标签支持 @用户、超链接、有序/无序列表、代码块、引用、分割线、加粗、斜体、下划线、删除线
		postContent := map[string]any{
			"zh_cn": map[string]any{
				"title": "",
				"content": [][]map[string]any{
					{
						{"tag": "md", "text": content},
					},
				},
			},
		}
		b, e := json.Marshal(postContent)
		if e != nil {
			// 转换失败降级为纯文本
			b, e = json.Marshal(map[string]string{"text": content})
			if e != nil {
				return "", "", e
			}
			return larkim.MsgTypeText, string(b), nil
		}
		return larkim.MsgTypePost, string(b), nil
	}

	b, e := json.Marshal(map[string]string{"text": content})
	if e != nil {
		return "", "", e
	}
	return larkim.MsgTypeText, string(b), nil
}

// containsMarkdown 检测内容是否包含常见 Markdown 语法。
func containsMarkdown(s string) bool {
	markers := []string{"**", "```", "# ", "## ", "### ", "`", "[", "*", "> ", "- ", "1. "}
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

func (c *FeishuChannel) handleMessageReceive(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if event == nil || event.Event == nil || event.Event.Message == nil {
		return nil
	}

	message := event.Event.Message
	sender := event.Event.Sender

	chatID := stringValue(message.ChatId)
	if chatID == "" {
		return nil
	}

	senderID := extractFeishuSenderID(sender)
	if senderID == "" {
		senderID = "unknown"
	}

	content := extractFeishuMessageContent(message)
	if content == "" {
		content = "[empty message]"
	}

	metadata := map[string]string{}
	if messageID := stringValue(message.MessageId); messageID != "" {
		metadata["message_id"] = messageID
	}
	if messageType := stringValue(message.MessageType); messageType != "" {
		metadata["message_type"] = messageType
	}
	if chatType := stringValue(message.ChatType); chatType != "" {
		metadata["chat_type"] = chatType
	}
	if sender != nil && sender.TenantKey != nil {
		metadata["tenant_key"] = *sender.TenantKey
	}

	// 提取引用消息的 ID（parent_id 是直接回复的消息，root_id 是整个回复链的根消息）
	parentID := stringValue(message.ParentId)
	rootID := stringValue(message.RootId)
	if parentID != "" {
		metadata["parent_id"] = parentID
	}
	if rootID != "" {
		metadata["root_id"] = rootID
	}

	// 如果是引用消息，获取被引用消息的内容并以 Quote Block 格式注入
	if parentID != "" {
		quotedContent := c.fetchQuotedMessage(ctx, parentID)
		if quotedContent != "" {
			content = quotedContent + "\n---\n" + content
		}
	}

	chatType := stringValue(message.ChatType)
	if chatType == "p2p" {
		metadata["peer_kind"] = "direct"
		metadata["peer_id"] = senderID
	} else {
		metadata["peer_kind"] = "group"
		metadata["peer_id"] = chatID
	}

	logger.InfoCF("feishu", "Feishu message received", map[string]any{
		"sender_id": senderID,
		"chat_id":   chatID,
		"parent_id": parentID,
		"preview":   utils.Truncate(content, 80),
	})

	c.HandleMessage(senderID, chatID, content, nil, metadata)
	return nil
}

func extractFeishuSenderID(sender *larkim.EventSender) string {
	if sender == nil || sender.SenderId == nil {
		return ""
	}

	if sender.SenderId.UserId != nil && *sender.SenderId.UserId != "" {
		return *sender.SenderId.UserId
	}
	if sender.SenderId.OpenId != nil && *sender.SenderId.OpenId != "" {
		return *sender.SenderId.OpenId
	}
	if sender.SenderId.UnionId != nil && *sender.SenderId.UnionId != "" {
		return *sender.SenderId.UnionId
	}

	return ""
}

func extractFeishuMessageContent(message *larkim.EventMessage) string {
	if message == nil || message.Content == nil || *message.Content == "" {
		return ""
	}

	if message.MessageType != nil && *message.MessageType == larkim.MsgTypeText {
		var textPayload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(*message.Content), &textPayload); err == nil {
			return textPayload.Text
		}
	}

	return *message.Content
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// fetchQuotedMessage 获取被引用消息的内容，并格式化为 Quote Block。
// 返回格式：[Feishu quote om_xxx] 发送者: 原消息内容
func (c *FeishuChannel) fetchQuotedMessage(ctx context.Context, messageID string) string {
	if messageID == "" {
		return ""
	}

	req := larkim.NewGetMessageReqBuilder().
		MessageId(messageID).
		Build()

	resp, err := c.client.Im.V1.Message.Get(ctx, req)
	if err != nil {
		logger.WarnCF("feishu", "Failed to fetch quoted message", map[string]any{
			"message_id": messageID,
			"error":      err.Error(),
		})
		return ""
	}

	if !resp.Success() || resp.Data == nil || len(resp.Data.Items) == 0 {
		logger.WarnCF("feishu", "Quoted message not found or API error", map[string]any{
			"message_id": messageID,
			"code":       resp.Code,
			"msg":        resp.Msg,
		})
		return ""
	}

	item := resp.Data.Items[0]

	// 提取发送者信息
	senderName := "未知用户"
	if item.Sender != nil {
		if item.Sender.Id != nil && *item.Sender.Id != "" {
			senderName = *item.Sender.Id
		}
	}

	// 提取消息内容
	quotedText := ""
	if item.Body != nil && item.Body.Content != nil {
		msgType := stringValue(item.MsgType)
		quotedText = extractMessageBodyContent(msgType, *item.Body.Content)
	}

	if quotedText == "" {
		quotedText = "[无法解析的消息类型]"
	}

	// 格式化为 Quote Block，便于 LLM 理解上下文
	return fmt.Sprintf("[Feishu quote %s] %s: %s", messageID, senderName, quotedText)
}

// extractMessageBodyContent 从消息体中提取文本内容。
func extractMessageBodyContent(msgType, content string) string {
	switch msgType {
	case "text":
		var textPayload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(content), &textPayload); err == nil {
			return textPayload.Text
		}
	case "post":
		// post 富文本格式，尝试提取纯文本
		var postPayload struct {
			ZhCN struct {
				Title   string `json:"title"`
				Content [][]struct {
					Tag  string `json:"tag"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"zh_cn"`
		}
		if err := json.Unmarshal([]byte(content), &postPayload); err == nil {
			var texts []string
			if postPayload.ZhCN.Title != "" {
				texts = append(texts, postPayload.ZhCN.Title)
			}
			for _, line := range postPayload.ZhCN.Content {
				for _, elem := range line {
					if elem.Text != "" {
						texts = append(texts, elem.Text)
					}
				}
			}
			return strings.Join(texts, " ")
		}
	case "image":
		return "[图片]"
	case "file":
		return "[文件]"
	case "audio":
		return "[语音]"
	case "video":
		return "[视频]"
	case "sticker":
		return "[表情包]"
	}
	return content
}
