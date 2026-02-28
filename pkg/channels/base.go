package channels

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
)

type Channel interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Send(ctx context.Context, msg bus.OutboundMessage) error
	IsRunning() bool
	IsAllowed(senderID string) bool
	AddReaction(ctx context.Context, messageID, emojiType string) error
}

type BaseChannel struct {
	config    any
	bus       *bus.MessageBus
	running   bool
	name      string
	allowList []string
}

func NewBaseChannel(name string, config any, bus *bus.MessageBus, allowList []string) *BaseChannel {
	return &BaseChannel{
		config:    config,
		bus:       bus,
		name:      name,
		allowList: allowList,
		running:   false,
	}
}

func (c *BaseChannel) Name() string {
	return c.name
}

func (c *BaseChannel) IsRunning() bool {
	return c.running
}

func (c *BaseChannel) IsAllowed(senderID string) bool {
	if len(c.allowList) == 0 {
		return true
	}

	// Extract parts from compound senderID like "123456|username"
	idPart := senderID
	userPart := ""
	if idx := strings.Index(senderID, "|"); idx > 0 {
		idPart = senderID[:idx]
		userPart = senderID[idx+1:]
	}

	for _, allowed := range c.allowList {
		// Strip leading "@" from allowed value for username matching
		trimmed := strings.TrimPrefix(allowed, "@")
		allowedID := trimmed
		allowedUser := ""
		if idx := strings.Index(trimmed, "|"); idx > 0 {
			allowedID = trimmed[:idx]
			allowedUser = trimmed[idx+1:]
		}

		// Support either side using "id|username" compound form.
		// This keeps backward compatibility with legacy Telegram allowlist entries.
		if senderID == allowed ||
			idPart == allowed ||
			senderID == trimmed ||
			idPart == trimmed ||
			idPart == allowedID ||
			(allowedUser != "" && senderID == allowedUser) ||
			(userPart != "" && (userPart == allowed || userPart == trimmed || userPart == allowedUser)) {
			return true
		}
	}

	return false
}

func (c *BaseChannel) HandleMessage(senderID, chatID, content string, media []string, metadata map[string]string) {
	if !c.IsAllowed(senderID) {
		return
	}

	// 自动确认机制：对收到的每一条消息，首先尝试回复一个 +1 表情
	// 这是一个“脊髓反射”动作，不经过 LLM，提升响应感知
	if messageID, ok := metadata["message_id"]; ok && messageID != "" {
		// 异步执行，不阻塞消息处理主流程
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			// 这里的 c 实际上是具体的 channel 实现（如 FeishuChannel）
			// 我们需要通过接口调用具体的 AddReaction 实现
			if ch, ok := any(c).(Channel); ok {
				_ = ch.AddReaction(ctx, messageID, "JIAYI")
			}
		}()
	}

	msg := bus.InboundMessage{
		Channel:  c.name,
		SenderID: senderID,
		ChatID:   chatID,
		Content:  content,
		Media:    media,
		Metadata: metadata,
	}

	c.bus.PublishInbound(msg)
}

func (c *BaseChannel) setRunning(running bool) {
	c.running = running
}

func (c *BaseChannel) AddReaction(ctx context.Context, messageID, emojiType string) error {
	return fmt.Errorf("AddReaction not implemented for channel %s", c.name)
}
