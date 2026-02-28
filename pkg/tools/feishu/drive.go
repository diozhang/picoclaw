//go:build amd64 || arm64 || riscv64 || mips64 || ppc64

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	larkdrive "github.com/larksuite/oapi-sdk-go/v3/service/drive/v1"
	larkdocx "github.com/larksuite/oapi-sdk-go/v3/service/docx/v1"

	"github.com/sipeed/picoclaw/pkg/tools"
)

// ============================================================
// feishu_get_doc_content
// ============================================================

// GetDocContentTool 获取飞书文档的纯文本内容，并列出文档中的图片/附件 token。
type GetDocContentTool struct {
	client *Client
}

func newGetDocContentTool(c *Client) *GetDocContentTool {
	return &GetDocContentTool{client: c}
}

func (t *GetDocContentTool) Name() string { return "feishu_get_doc_content" }

func (t *GetDocContentTool) Description() string {
	return "获取飞书文档（docx）的内容。返回文档纯文本，并列出文档中所有图片和附件的 file_token 及临时下载 URL（24 小时有效）。" +
		"document_id 可从文档 URL 中获取（如 https://xxx.feishu.cn/docx/DOCUMENT_ID）。" +
		"如需下载图片/附件，使用返回的 tmp_download_url 或调用 feishu_download_doc_media 工具。"
}

func (t *GetDocContentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"document_id": map[string]any{
				"type":        "string",
				"description": "文档 ID，从文档 URL 中获取",
			},
		},
		"required": []string{"document_id"},
	}
}

func (t *GetDocContentTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	documentID, _ := args["document_id"].(string)
	if documentID == "" {
		return tools.ErrorResult("document_id 不能为空")
	}

	// 1. 获取文档纯文本内容
	rawResp, err := t.client.lark.Docx.V1.Document.RawContent(ctx,
		larkdocx.NewRawContentDocumentReqBuilder().
			DocumentId(documentID).
			Lang(0). // 0 = 中文
			Build())
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("获取文档内容失败: %v", err))
	}
	if !rawResp.Success() {
		return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", rawResp.Code, rawResp.Msg))
	}

	textContent := ""
	if rawResp.Data != nil {
		textContent = strVal(rawResp.Data.Content)
	}

	// 2. 遍历 Block 列表，收集图片和附件 token
	mediaTokens := collectMediaTokens(ctx, t.client, documentID)

	// 3. 批量获取临时下载 URL
	mediaItems := fetchTmpDownloadURLs(ctx, t.client, mediaTokens)

	result := map[string]any{
		"document_id": documentID,
		"content":     textContent,
		"media":       mediaItems,
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return tools.SilentResult(string(out))
}

// ============================================================
// feishu_download_doc_media
// ============================================================

// DownloadDocMediaTool 根据 file_token 下载文档中的图片或附件，保存到工作区。
type DownloadDocMediaTool struct {
	client    *Client
	workspace string
}

func newDownloadDocMediaTool(c *Client, workspace string) *DownloadDocMediaTool {
	return &DownloadDocMediaTool{client: c, workspace: workspace}
}

func (t *DownloadDocMediaTool) Name() string { return "feishu_download_doc_media" }

func (t *DownloadDocMediaTool) Description() string {
	return "根据 file_token 下载飞书文档中的图片或附件，保存到本地工作区并返回文件路径。" +
		"file_token 可从 feishu_get_doc_content 返回的 media 列表中获取。"
}

func (t *DownloadDocMediaTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_token": map[string]any{
				"type":        "string",
				"description": "文件 token，从 feishu_get_doc_content 的 media 列表中获取",
			},
			"filename": map[string]any{
				"type":        "string",
				"description": "保存的文件名（可选），不填则自动生成",
			},
		},
		"required": []string{"file_token"},
	}
}

func (t *DownloadDocMediaTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	fileToken, _ := args["file_token"].(string)
	filename, _ := args["filename"].(string)

	if fileToken == "" {
		return tools.ErrorResult("file_token 不能为空")
	}

	req := larkdrive.NewDownloadMediaReqBuilder().
		FileToken(fileToken).
		Build()

	resp, err := t.client.lark.Drive.V1.Media.Download(ctx, req)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("下载媒体文件失败: %v", err))
	}
	if !resp.Success() {
		return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", resp.Code, resp.Msg))
	}

	saveName := filename
	if saveName == "" {
		if resp.FileName != "" {
			saveName = resp.FileName
		} else {
			saveName = fmt.Sprintf("doc_media_%d.bin", time.Now().UnixNano())
		}
	}

	// 文档媒体统一存放到 downloads/feishu/docs/ 子目录
	savePath := filepath.Join(t.workspace, "downloads", "feishu", "docs", saveName)
	if err := saveStream(resp.File, savePath); err != nil {
		return tools.ErrorResult(fmt.Sprintf("保存文件失败: %v", err))
	}

	return tools.SilentResult(fmt.Sprintf("文件已保存到: %s", savePath))
}

// ============================================================
// 内部工具函数
// ============================================================

// mediaItem 表示文档中的一个媒体资源。
type mediaItem struct {
	FileToken      string `json:"file_token"`
	Type           string `json:"type"` // "image" 或 "file"
	TmpDownloadURL string `json:"tmp_download_url,omitempty"`
}

// collectMediaTokens 遍历文档所有 Block，收集图片和附件的 file_token。
func collectMediaTokens(ctx context.Context, c *Client, documentID string) []mediaItem {
	var items []mediaItem

	req := larkdocx.NewListDocumentBlockReqBuilder().
		DocumentId(documentID).
		PageSize(200).
		Build()

	resp, err := c.lark.Docx.V1.DocumentBlock.List(ctx, req)
	if err != nil || !resp.Success() {
		return items
	}
	if resp.Data == nil {
		return items
	}

	for _, block := range resp.Data.Items {
		if block.Image != nil && block.Image.Token != nil {
			items = append(items, mediaItem{
				FileToken: *block.Image.Token,
				Type:      "image",
			})
		}
		if block.File != nil && block.File.Token != nil {
			items = append(items, mediaItem{
				FileToken: *block.File.Token,
				Type:      "file",
			})
		}
	}

	return items
}

// fetchTmpDownloadURLs 批量获取 file_token 对应的临时下载 URL。
func fetchTmpDownloadURLs(ctx context.Context, c *Client, items []mediaItem) []mediaItem {
	if len(items) == 0 {
		return items
	}

	tokens := make([]string, 0, len(items))
	for _, item := range items {
		tokens = append(tokens, item.FileToken)
	}

	req := larkdrive.NewBatchGetTmpDownloadUrlMediaReqBuilder().
		FileTokens(tokens).
		Build()

	resp, err := c.lark.Drive.V1.Media.BatchGetTmpDownloadUrl(ctx, req)
	if err != nil || !resp.Success() || resp.Data == nil {
		return items
	}

	// 建立 token → url 的映射
	urlMap := make(map[string]string, len(resp.Data.TmpDownloadUrls))
	for _, u := range resp.Data.TmpDownloadUrls {
		if u.FileToken != nil && u.TmpDownloadUrl != nil {
			urlMap[*u.FileToken] = *u.TmpDownloadUrl
		}
	}

	for i := range items {
		if url, ok := urlMap[items[i].FileToken]; ok {
			items[i].TmpDownloadURL = url
		}
	}

	return items
}
