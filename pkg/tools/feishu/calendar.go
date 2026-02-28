//go:build amd64 || arm64 || riscv64 || mips64 || ppc64

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	larkcalendar "github.com/larksuite/oapi-sdk-go/v3/service/calendar/v4"

	"github.com/sipeed/picoclaw/pkg/tools"
)

// ============================================================
// feishu_list_events
// ============================================================

// ListEventsTool 查询日历事件列表。
type ListEventsTool struct {
	client *Client
}

func newListEventsTool(c *Client) *ListEventsTool {
	return &ListEventsTool{client: c}
}

func (t *ListEventsTool) Name() string { return "feishu_list_events" }

func (t *ListEventsTool) Description() string {
	return "查询飞书日历中的事件（日程）列表，支持按时间范围过滤。" +
		"calendar_id 为日历 ID，留空则自动使用当前用户的主日历（primary）。" +
		"start_time 和 end_time 为 Unix 时间戳（秒）。"
}

func (t *ListEventsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"calendar_id": map[string]any{
				"type":        "string",
				"description": "日历 ID，留空则使用主日历",
			},
			"start_time": map[string]any{
				"type":        "string",
				"description": "查询起始时间，Unix 时间戳（秒），默认为当前时间",
			},
			"end_time": map[string]any{
				"type":        "string",
				"description": "查询结束时间，Unix 时间戳（秒），默认为 start_time + 7 天",
			},
			"page_size": map[string]any{
				"type":        "integer",
				"description": "每页事件数量，最大 500，默认 50",
			},
		},
	}
}

func (t *ListEventsTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	calendarID, _ := args["calendar_id"].(string)
	startTimeStr, _ := args["start_time"].(string)
	endTimeStr, _ := args["end_time"].(string)

	pageSize := 50
	if v, ok := args["page_size"].(float64); ok && v > 0 {
		pageSize = int(v)
		if pageSize > 500 {
			pageSize = 500
		}
	}

	// 默认时间范围：当前时间到 7 天后
	now := time.Now()
	if startTimeStr == "" {
		startTimeStr = fmt.Sprintf("%d", now.Unix())
	}
	if endTimeStr == "" {
		endTimeStr = fmt.Sprintf("%d", now.Add(7*24*time.Hour).Unix())
	}

	// 若未指定日历 ID，先获取主日历
	if calendarID == "" {
		primaryResp, err := t.client.lark.Calendar.V4.Calendar.Primary(ctx,
			larkcalendar.NewPrimaryCalendarReqBuilder().Build())
		if err != nil {
			return tools.ErrorResult(fmt.Sprintf("获取主日历失败: %v", err))
		}
		if !primaryResp.Success() {
			return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", primaryResp.Code, primaryResp.Msg))
		}
		if primaryResp.Data != nil && len(primaryResp.Data.Calendars) > 0 &&
			primaryResp.Data.Calendars[0].Calendar != nil {
			calendarID = strVal(primaryResp.Data.Calendars[0].Calendar.CalendarId)
		}
		if calendarID == "" {
			return tools.ErrorResult("无法获取主日历 ID，请手动指定 calendar_id")
		}
	}

	req := larkcalendar.NewListCalendarEventReqBuilder().
		CalendarId(calendarID).
		StartTime(startTimeStr).
		EndTime(endTimeStr).
		PageSize(pageSize).
		Build()

	resp, err := t.client.lark.Calendar.V4.CalendarEvent.List(ctx, req)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("获取日程列表失败: %v", err))
	}
	if !resp.Success() {
		return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", resp.Code, resp.Msg))
	}

	if resp.Data == nil || len(resp.Data.Items) == 0 {
		return tools.SilentResult("该时间范围内没有日程")
	}

	type eventSummary struct {
		EventID     string `json:"event_id"`
		Summary     string `json:"summary"`
		Description string `json:"description"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
		Location    string `json:"location,omitempty"`
		Status      string `json:"status,omitempty"`
		AppLink     string `json:"app_link,omitempty"`
	}

	events := make([]eventSummary, 0, len(resp.Data.Items))
	for _, e := range resp.Data.Items {
		startTS := ""
		endTS := ""
		if e.StartTime != nil {
			startTS = strVal(e.StartTime.Timestamp)
		}
		if e.EndTime != nil {
			endTS = strVal(e.EndTime.Timestamp)
		}
		location := ""
		if e.Location != nil {
			location = strVal(e.Location.Name)
		}
		events = append(events, eventSummary{
			EventID:     strVal(e.EventId),
			Summary:     strVal(e.Summary),
			Description: strVal(e.Description),
			StartTime:   startTS,
			EndTime:     endTS,
			Location:    location,
			Status:      strVal(e.Status),
			AppLink:     strVal(e.AppLink),
		})
	}

	out, _ := json.MarshalIndent(map[string]any{
		"calendar_id": calendarID,
		"total":       len(events),
		"events":      events,
	}, "", "  ")
	return tools.SilentResult(string(out))
}

// ============================================================
// feishu_create_event
// ============================================================

// CreateEventTool 在飞书日历中创建日程。
type CreateEventTool struct {
	client *Client
}

func newCreateEventTool(c *Client) *CreateEventTool {
	return &CreateEventTool{client: c}
}

func (t *CreateEventTool) Name() string { return "feishu_create_event" }

func (t *CreateEventTool) Description() string {
	return "在飞书日历中创建日程。" +
		"start_time 和 end_time 为 Unix 时间戳（秒）。" +
		"attendees 为参与人的 open_id 列表（可选）。" +
		"calendar_id 留空则使用主日历。"
}

func (t *CreateEventTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "日程标题",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "日程描述（可选）",
			},
			"start_time": map[string]any{
				"type":        "string",
				"description": "日程开始时间，Unix 时间戳（秒）",
			},
			"end_time": map[string]any{
				"type":        "string",
				"description": "日程结束时间，Unix 时间戳（秒）",
			},
			"timezone": map[string]any{
				"type":        "string",
				"description": "时区，默认 Asia/Shanghai",
			},
			"location": map[string]any{
				"type":        "string",
				"description": "地点名称（可选）",
			},
			"attendees": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "参与人 open_id 列表（可选）",
			},
			"calendar_id": map[string]any{
				"type":        "string",
				"description": "日历 ID，留空则使用主日历",
			},
		},
		"required": []string{"summary", "start_time", "end_time"},
	}
}

func (t *CreateEventTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	summary, _ := args["summary"].(string)
	description, _ := args["description"].(string)
	startTimeStr, _ := args["start_time"].(string)
	endTimeStr, _ := args["end_time"].(string)
	timezone, _ := args["timezone"].(string)
	location, _ := args["location"].(string)
	calendarID, _ := args["calendar_id"].(string)
	attendeeIDs := toStringSlice(args["attendees"])

	if summary == "" {
		return tools.ErrorResult("summary 不能为空")
	}
	if startTimeStr == "" || endTimeStr == "" {
		return tools.ErrorResult("start_time 和 end_time 不能为空")
	}
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}

	// 若未指定日历 ID，先获取主日历
	if calendarID == "" {
		primaryResp, err := t.client.lark.Calendar.V4.Calendar.Primary(ctx,
			larkcalendar.NewPrimaryCalendarReqBuilder().Build())
		if err != nil {
			return tools.ErrorResult(fmt.Sprintf("获取主日历失败: %v", err))
		}
		if !primaryResp.Success() {
			return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", primaryResp.Code, primaryResp.Msg))
		}
		if primaryResp.Data != nil && len(primaryResp.Data.Calendars) > 0 &&
			primaryResp.Data.Calendars[0].Calendar != nil {
			calendarID = strVal(primaryResp.Data.Calendars[0].Calendar.CalendarId)
		}
		if calendarID == "" {
			return tools.ErrorResult("无法获取主日历 ID，请手动指定 calendar_id")
		}
	}

	startTime := larkcalendar.NewTimeInfoBuilder().
		Timestamp(startTimeStr).
		Timezone(timezone).
		Build()
	endTime := larkcalendar.NewTimeInfoBuilder().
		Timestamp(endTimeStr).
		Timezone(timezone).
		Build()

	eventBuilder := larkcalendar.NewCalendarEventBuilder().
		Summary(summary).
		StartTime(startTime).
		EndTime(endTime)

	if description != "" {
		eventBuilder = eventBuilder.Description(description)
	}
	if location != "" {
		loc := larkcalendar.NewEventLocationBuilder().Name(location).Build()
		eventBuilder = eventBuilder.Location(loc)
	}

	// 构建参与人列表
	if len(attendeeIDs) > 0 {
		attendees := make([]*larkcalendar.CalendarEventAttendee, 0, len(attendeeIDs))
		for _, openID := range attendeeIDs {
			attendees = append(attendees, larkcalendar.NewCalendarEventAttendeeBuilder().
				Type("user").
				UserId(openID).
				Build())
		}
		eventBuilder = eventBuilder.Attendees(attendees)
	}

	req := larkcalendar.NewCreateCalendarEventReqBuilder().
		CalendarId(calendarID).
		CalendarEvent(eventBuilder.Build()).
		Build()

	resp, err := t.client.lark.Calendar.V4.CalendarEvent.Create(ctx, req)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("创建日程失败: %v", err))
	}
	if !resp.Success() {
		return tools.ErrorResult(fmt.Sprintf("飞书 API 错误: code=%d msg=%s", resp.Code, resp.Msg))
	}

	eventID := ""
	appLink := ""
	if resp.Data != nil && resp.Data.Event != nil {
		eventID = strVal(resp.Data.Event.EventId)
		appLink = strVal(resp.Data.Event.AppLink)
	}

	return tools.SilentResult(fmt.Sprintf("日程创建成功，event_id=%s，app_link=%s", eventID, appLink))
}
