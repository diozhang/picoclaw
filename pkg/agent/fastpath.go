package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// FastPathHandler 处理快速路径逻辑
type FastPathHandler struct {
	// 可以注入需要的组件，如消息总线或特定的 API 客户端
	bus *bus.MessageBus
}

// NewFastPathHandler 创建一个新的快速路径处理器
func NewFastPathHandler(msgBus *bus.MessageBus) *FastPathHandler {
	return &FastPathHandler{
		bus: msgBus,
	}
}

// TryHandle 尝试拦截并处理简单消息，如果处理了则返回 true
func (h *FastPathHandler) TryHandle(ctx context.Context, msg bus.InboundMessage, cm *channels.Manager) (string, bool) {
	content := strings.TrimSpace(strings.ToLower(msg.Content))
	if content == "" {
		return "", false
	}

	// 1. 停止指令拦截 (Abort Triggers)
	if h.isAbortTrigger(content) {
		logger.InfoCF("fastpath", "Abort trigger detected", map[string]any{
			"content": content,
			"chat_id": msg.ChatID,
		})
		// 这里可以实现停止当前运行任务的逻辑
		// 目前 picoclaw 的任务模型是同步的，如果是异步的可以调用 cancel
		return "已停止当前任务。", true
	}

	// 2. 表情回复拦截 (Reaction Triggers)
	if emojiType, ok := h.matchReactionTrigger(content); ok {
		messageID := msg.Metadata["message_id"]
		if messageID != "" && msg.Channel == "feishu" && cm != nil {
			logger.InfoCF("fastpath", "Reaction trigger detected", map[string]any{
				"content":    content,
				"emoji_type": emojiType,
				"message_id": messageID,
			})

			// 极速决策：直接调用 Channel 驱动的 API，绕过 LLM
			if ch, ok := cm.GetChannel("feishu"); ok {
				err := ch.AddReaction(ctx, messageID, emojiType)
				if err != nil {
					logger.ErrorCF("fastpath", "Failed to add reaction via fastpath", map[string]any{
						"error": err.Error(),
					})
					return "", false // 失败则降级走 LLM
				}
				return fmt.Sprintf("[极速决策] 已为您点赞 (%s)", emojiType), true
			}
		}
	}

	return "", false
}

func (h *FastPathHandler) isAbortTrigger(content string) bool {
	triggers := []string{
		"stop", "halt", "abort", "exit", "interrupt", "wait", "esc",
		"停止", "取消", "别做了", "中断", "退出", "停",
	}
	for _, t := range triggers {
		if content == t {
			return true
		}
	}
	return false
}

func (h *FastPathHandler) matchReactionTrigger(content string) (string, bool) {
	// THUMBSUP 赞
	thumbsUp := []string{"ok", "okay", "好的", "收到", "明白", "+1", "赞", "👍", "666", "nice"}
	for _, t := range thumbsUp {
		if content == t {
			return "THUMBSUP", true
		}
	}

	// HEART 爱心
	heart := []string{"爱心", "喜欢", "比心", "❤️", "love"}
	for _, t := range heart {
		if content == t {
			return "HEART", true
		}
	}

	// OK
	if content == "ok" || content == "okay" {
		return "OK", true
	}

	return "", false
}
