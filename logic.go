// logic.go contains pure business logic shared between the wasip1 plugin
// and the host-side test build. No pdk imports here.

package main

import (
	"encoding/json"
	"fmt"
)

// validateStatus returns an error if s is not in the provided valid statuses list.
func validateStatus(s string, validStatuses []string) error {
	for _, v := range validStatuses {
		if s == v {
			return nil
		}
	}
	return fmt.Errorf("invalid status %q: not a defined board column", s)
}

// buildListTasksQuery constructs the SELECT SQL and args slice for list_tasks.
func buildListTasksQuery(params ListTasksParams) (string, []any) {
	sql := "SELECT id, title, description, status, priority, due_date, project_id, type_id, assignee_id, created_at, updated_at FROM tasks"
	var args []any
	var conditions []string

	if params.StatusFilter != "" {
		args = append(args, params.StatusFilter)
		conditions = append(conditions, fmt.Sprintf("status = ?%d", len(args)))
	}
	if params.PriorityMin != 0 {
		args = append(args, params.PriorityMin)
		conditions = append(conditions, fmt.Sprintf("priority >= ?%d", len(args)))
	}
	if params.PriorityMax != 0 {
		args = append(args, params.PriorityMax)
		conditions = append(conditions, fmt.Sprintf("priority <= ?%d", len(args)))
	}
	if params.ProjectID != nil {
		args = append(args, *params.ProjectID)
		conditions = append(conditions, fmt.Sprintf("project_id = ?%d", len(args)))
	}
	if params.TypeID != nil {
		args = append(args, *params.TypeID)
		conditions = append(conditions, fmt.Sprintf("type_id = ?%d", len(args)))
	}
	if params.AssigneeID != nil {
		args = append(args, *params.AssigneeID)
		conditions = append(conditions, fmt.Sprintf("assignee_id = ?%d", len(args)))
	}

	if len(conditions) > 0 {
		sql += " WHERE "
		for i, c := range conditions {
			if i > 0 {
				sql += " AND "
			}
			sql += c
		}
	}

	sql += " ORDER BY priority DESC, created_at ASC"

	if params.Limit > 0 {
		args = append(args, params.Limit)
		sql += fmt.Sprintf(" LIMIT ?%d", len(args))
	}

	return sql, args
}

// ---- Generic row helpers ----

func makeColIdx(columns []string) map[string]int {
	idx := make(map[string]int, len(columns))
	for i, c := range columns {
		idx[c] = i
	}
	return idx
}

func getStr(colIdx map[string]int, row []any, col string) string {
	idx, ok := colIdx[col]
	if !ok || idx >= len(row) || row[idx] == nil {
		return ""
	}
	switch v := row[idx].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func getInt64(colIdx map[string]int, row []any, col string) int64 {
	idx, ok := colIdx[col]
	if !ok || idx >= len(row) || row[idx] == nil {
		return 0
	}
	switch v := row[idx].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	default:
		return 0
	}
}

func getNullableInt64(colIdx map[string]int, row []any, col string) *int64 {
	idx, ok := colIdx[col]
	if !ok || idx >= len(row) || row[idx] == nil {
		return nil
	}
	v := getInt64(colIdx, row, col)
	return &v
}

func getBool(colIdx map[string]int, row []any, col string) bool {
	return getInt64(colIdx, row, col) != 0
}

// ---- Row mappers ----

// rowToTask converts a single db result row into a Task.
func rowToTask(columns []string, row []any) (Task, error) {
	ci := makeColIdx(columns)
	return Task{
		ID:          getInt64(ci, row, "id"),
		Title:       getStr(ci, row, "title"),
		Description: getStr(ci, row, "description"),
		Status:      getStr(ci, row, "status"),
		Priority:    int(getInt64(ci, row, "priority")),
		DueDate:     getStr(ci, row, "due_date"),
		ProjectID:   getNullableInt64(ci, row, "project_id"),
		TypeID:      getNullableInt64(ci, row, "type_id"),
		AssigneeID:  getNullableInt64(ci, row, "assignee_id"),
		CreatedAt:   getStr(ci, row, "created_at"),
		UpdatedAt:   getStr(ci, row, "updated_at"),
	}, nil
}

// rowToTaskOverviewItem converts a single db result row into a TaskOverviewItem.
func rowToTaskOverviewItem(columns []string, row []any) (TaskOverviewItem, error) {
	ci := makeColIdx(columns)
	return TaskOverviewItem{
		ID:        getInt64(ci, row, "id"),
		Title:     getStr(ci, row, "title"),
		Status:    getStr(ci, row, "status"),
		Priority:  int(getInt64(ci, row, "priority")),
		ProjectID: getNullableInt64(ci, row, "project_id"),
		CreatedAt: getStr(ci, row, "created_at"),
	}, nil
}

// rowToBoardColumnDef converts a single db result row into a BoardColumnDef.
func rowToBoardColumnDef(columns []string, row []any) (BoardColumnDef, error) {
	ci := makeColIdx(columns)
	return BoardColumnDef{
		ID:        getInt64(ci, row, "id"),
		Name:      getStr(ci, row, "name"),
		Label:     getStr(ci, row, "label"),
		Position:  int(getInt64(ci, row, "position")),
		Color:     getStr(ci, row, "color"),
		ProjectID: getNullableInt64(ci, row, "project_id"),
		IsDefault: getBool(ci, row, "is_default"),
		CreatedAt: getStr(ci, row, "created_at"),
	}, nil
}

// rowToTaskType converts a single db result row into a TaskType.
func rowToTaskType(columns []string, row []any) (TaskType, error) {
	ci := makeColIdx(columns)
	return TaskType{
		ID:        getInt64(ci, row, "id"),
		Name:      getStr(ci, row, "name"),
		Label:     getStr(ci, row, "label"),
		Color:     getStr(ci, row, "color"),
		Icon:      getStr(ci, row, "icon"),
		ProjectID: getNullableInt64(ci, row, "project_id"),
		CreatedAt: getStr(ci, row, "created_at"),
	}, nil
}

// rowToComment converts a single db result row into a Comment.
func rowToComment(columns []string, row []any) (Comment, error) {
	ci := makeColIdx(columns)
	return Comment{
		ID:         getInt64(ci, row, "id"),
		TaskID:     getInt64(ci, row, "task_id"),
		AuthorType: getStr(ci, row, "author_type"),
		AuthorID:   getNullableInt64(ci, row, "author_id"),
		AuthorName: getStr(ci, row, "author_name"),
		Body:       getStr(ci, row, "body"),
		CreatedAt:  getStr(ci, row, "created_at"),
	}, nil
}
