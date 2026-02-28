//go:build amd64 || arm64 || riscv64 || mips64 || ppc64

package feishu

import (
	"context"
	"encoding/json"
	"fmt"

	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"

	"github.com/sipeed/picoclaw/pkg/tools"
)

// ============================================================
// feishu_get_user
// ============================================================

// GetUserTool 根据用户 ID 获取用户详情。
type GetUserTool struct {
	client *Client
}

func newGetUserTool(c *Client) *GetUserTool {
	return &GetUserTool{client: c}
}

func (t *GetUserTool) Name() string { return "feishu_get_user" }

func (t *GetUserTool) Description() string {
	return "根据用户 ID 获取飞书用户的详细信息，包括姓名、邮箱、手机号、部门、职务、工号等。" +
		"user_id_type 指定 ID 类型：open_id（默认）、user_id、union_id。"
}

func (t *GetUserTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"user_id": map[string]any{
				"type":        "string",
				"description": "用户 ID，与 user_id_type 对应",
			},
			"user_id_type": map[string]any{
				"type":        "string",
				"description": "用户 ID 类型：open_id（默认）、user_id、union_id",
				"enum":        []string{"open_id", "user_id", "union_id"},
			},
		},
		"required": []string{"user_id"},
	}
}

func (t *GetUserTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	userID, _ := args["user_id"].(string)
	userIDType, _ := args["user_id_type"].(string)
	if userID == "" {
		return tools.ErrorResult("user_id 不能为空")
	}
	if userIDType == "" {
		userIDType = "open_id"
	}

	req := larkcontact.NewGetUserReqBuilder().
		UserId(userID).
		UserIdType(userIDType).
		Build()

	resp, err := t.client.lark.Contact.V3.User.Get(ctx, req)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("获取用户失败: %v", err))
	}
	if !resp.Success() {
		return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", resp.Code, resp.Msg))
	}
	if resp.Data == nil || resp.Data.User == nil {
		return tools.SilentResult("未找到该用户")
	}

	u := resp.Data.User
	result := map[string]any{
		"user_id":     strVal(u.UserId),
		"open_id":     strVal(u.OpenId),
		"union_id":    strVal(u.UnionId),
		"name":        strVal(u.Name),
		"en_name":     strVal(u.EnName),
		"email":       strVal(u.Email),
		"mobile":      strVal(u.Mobile),
		"job_title":   strVal(u.JobTitle),
		"employee_no": strVal(u.EmployeeNo),
		"city":        strVal(u.City),
		"description": strVal(u.Description),
		"departments": u.DepartmentIds,
	}
	if u.Status != nil {
		result["status"] = map[string]any{
			"is_frozen":   u.Status.IsFrozen,
			"is_resigned": u.Status.IsResigned,
			"is_activated": u.Status.IsActivated,
		}
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return tools.SilentResult(string(out))
}

// ============================================================
// feishu_search_user
// ============================================================

// SearchUserTool 通过关键词搜索用户（需要 contact:user.base:readonly 权限）。
// 飞书通讯录 v3 没有独立的 user search 接口，使用 List 接口按部门列出，
// 或通过 BatchGetId 按邮箱/手机号精确查找。
// 此处实现按邮箱/手机号精确查找（BatchGetId）。
type SearchUserTool struct {
	client *Client
}

func newSearchUserTool(c *Client) *SearchUserTool {
	return &SearchUserTool{client: c}
}

func (t *SearchUserTool) Name() string { return "feishu_search_user" }

func (t *SearchUserTool) Description() string {
	return "通过邮箱或手机号精确查找飞书用户，返回对应的用户 ID 信息。" +
		"emails 和 mobiles 至少填写一个，支持同时传入多个。"
}

func (t *SearchUserTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"emails": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "邮箱列表，最多 50 个",
			},
			"mobiles": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "手机号列表（含国际区号，如 +8613800138000），最多 50 个",
			},
		},
	}
}

func (t *SearchUserTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	emails := toStringSlice(args["emails"])
	mobiles := toStringSlice(args["mobiles"])

	if len(emails) == 0 && len(mobiles) == 0 {
		return tools.ErrorResult("emails 和 mobiles 至少填写一个")
	}

	bodyBuilder := larkcontact.NewBatchGetIdUserReqBodyBuilder()
	if len(emails) > 0 {
		bodyBuilder = bodyBuilder.Emails(emails)
	}
	if len(mobiles) > 0 {
		bodyBuilder = bodyBuilder.Mobiles(mobiles)
	}

	req := larkcontact.NewBatchGetIdUserReqBuilder().
		UserIdType("open_id").
		Body(bodyBuilder.Build()).
		Build()

	resp, err := t.client.lark.Contact.V3.User.BatchGetId(ctx, req)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("搜索用户失败: %v", err))
	}
	if !resp.Success() {
		return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", resp.Code, resp.Msg))
	}

	if resp.Data == nil || len(resp.Data.UserList) == 0 {
		return tools.SilentResult("未找到匹配的用户")
	}

	type userIDResult struct {
		UserID string `json:"user_id"`
		Mobile string `json:"mobile,omitempty"`
		Email  string `json:"email,omitempty"`
	}

	results := make([]userIDResult, 0, len(resp.Data.UserList))
	for _, u := range resp.Data.UserList {
		results = append(results, userIDResult{
			UserID: strVal(u.UserId),
			Mobile: strVal(u.Mobile),
			Email:  strVal(u.Email),
		})
	}

	out, _ := json.MarshalIndent(map[string]any{
		"total": len(results),
		"users": results,
	}, "", "  ")
	return tools.SilentResult(string(out))
}

// ============================================================
// feishu_get_department
// ============================================================

// GetDepartmentTool 获取部门详情。
type GetDepartmentTool struct {
	client *Client
}

func newGetDepartmentTool(c *Client) *GetDepartmentTool {
	return &GetDepartmentTool{client: c}
}

func (t *GetDepartmentTool) Name() string { return "feishu_get_department" }

func (t *GetDepartmentTool) Description() string {
	return "获取飞书部门的详细信息，包括部门名称、父部门、负责人等。" +
		"department_id 为部门 ID，department_id_type 指定 ID 类型：open_department_id（默认）或 department_id。"
}

func (t *GetDepartmentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"department_id": map[string]any{
				"type":        "string",
				"description": "部门 ID",
			},
			"department_id_type": map[string]any{
				"type":        "string",
				"description": "部门 ID 类型：open_department_id（默认）、department_id",
				"enum":        []string{"open_department_id", "department_id"},
			},
		},
		"required": []string{"department_id"},
	}
}

func (t *GetDepartmentTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	departmentID, _ := args["department_id"].(string)
	departmentIDType, _ := args["department_id_type"].(string)
	if departmentID == "" {
		return tools.ErrorResult("department_id 不能为空")
	}
	if departmentIDType == "" {
		departmentIDType = "open_department_id"
	}

	req := larkcontact.NewGetDepartmentReqBuilder().
		DepartmentId(departmentID).
		DepartmentIdType(departmentIDType).
		Build()

	resp, err := t.client.lark.Contact.V3.Department.Get(ctx, req)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("获取部门失败: %v", err))
	}
	if !resp.Success() {
		return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", resp.Code, resp.Msg))
	}
	if resp.Data == nil || resp.Data.Department == nil {
		return tools.SilentResult("未找到该部门")
	}

	d := resp.Data.Department
	result := map[string]any{
		"name":                 strVal(d.Name),
		"department_id":        strVal(d.DepartmentId),
		"open_department_id":   strVal(d.OpenDepartmentId),
		"parent_department_id": strVal(d.ParentDepartmentId),
		"leader_user_id":       strVal(d.LeaderUserId),
		"member_count":         intVal(d.MemberCount),
	}
	if d.Status != nil {
		result["is_deleted"] = boolVal(d.Status.IsDeleted)
	}

	out, _ := json.MarshalIndent(result, "", "  ")
	return tools.SilentResult(string(out))
}

// ============================================================
// feishu_list_department_users
// ============================================================

// ListDepartmentUsersTool 获取部门下的用户列表。
type ListDepartmentUsersTool struct {
	client *Client
}

func newListDepartmentUsersTool(c *Client) *ListDepartmentUsersTool {
	return &ListDepartmentUsersTool{client: c}
}

func (t *ListDepartmentUsersTool) Name() string { return "feishu_list_department_users" }

func (t *ListDepartmentUsersTool) Description() string {
	return "获取飞书指定部门下的用户列表，返回用户基本信息（姓名、ID、邮箱、职务等）。" +
		"page_size 最大 50，默认 20。"
}

func (t *ListDepartmentUsersTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"department_id": map[string]any{
				"type":        "string",
				"description": "部门 ID",
			},
			"department_id_type": map[string]any{
				"type":        "string",
				"description": "部门 ID 类型：open_department_id（默认）、department_id",
				"enum":        []string{"open_department_id", "department_id"},
			},
			"page_size": map[string]any{
				"type":        "integer",
				"description": "每页用户数量，最大 50，默认 20",
			},
		},
		"required": []string{"department_id"},
	}
}

func (t *ListDepartmentUsersTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	departmentID, _ := args["department_id"].(string)
	departmentIDType, _ := args["department_id_type"].(string)
	if departmentID == "" {
		return tools.ErrorResult("department_id 不能为空")
	}
	if departmentIDType == "" {
		departmentIDType = "open_department_id"
	}

	pageSize := 20
	if v, ok := args["page_size"].(float64); ok && v > 0 {
		pageSize = int(v)
		if pageSize > 50 {
			pageSize = 50
		}
	}

	req := larkcontact.NewListUserReqBuilder().
		DepartmentId(departmentID).
		DepartmentIdType(departmentIDType).
		UserIdType("open_id").
		PageSize(pageSize).
		Build()

	resp, err := t.client.lark.Contact.V3.User.List(ctx, req)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("获取部门用户列表失败: %v", err))
	}
	if !resp.Success() {
		return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", resp.Code, resp.Msg))
	}

	if resp.Data == nil || len(resp.Data.Items) == 0 {
		return tools.SilentResult("该部门暂无成员")
	}

	type userSummary struct {
		OpenID    string `json:"open_id"`
		UserID    string `json:"user_id"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		Mobile    string `json:"mobile"`
		JobTitle  string `json:"job_title"`
		EmployeeNo string `json:"employee_no"`
	}

	users := make([]userSummary, 0, len(resp.Data.Items))
	for _, u := range resp.Data.Items {
		users = append(users, userSummary{
			OpenID:    strVal(u.OpenId),
			UserID:    strVal(u.UserId),
			Name:      strVal(u.Name),
			Email:     strVal(u.Email),
			Mobile:    strVal(u.Mobile),
			JobTitle:  strVal(u.JobTitle),
			EmployeeNo: strVal(u.EmployeeNo),
		})
	}

	out, _ := json.MarshalIndent(map[string]any{
		"total": len(users),
		"users": users,
	}, "", "  ")
	return tools.SilentResult(string(out))
}

// ============================================================
// 内部工具函数
// ============================================================

// toStringSlice 将 interface{} 转换为 []string。
func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && s != "" {
			result = append(result, s)
		}
	}
	return result
}

// intVal 安全解引用 *int 指针。
func intVal(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
