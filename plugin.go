//go:build wasip1

package main

import (
	"encoding/json"
	"fmt"

	pdk "github.com/extism/go-pdk"
)

// ---- Host function imports ----

//go:wasmimport mc_db query
func hostDBQuery(offset uint64) uint64

//go:wasmimport mc_db exec
func hostDBExec(offset uint64) uint64

//go:wasmimport mc_event emit
func hostEventEmit(offset uint64) uint64

//go:wasmimport mc_time now
func hostTimeNow(offset uint64) uint64

// ---- Host function helpers ----

// hostErrorResponse detects error responses from host functions.
// The host returns {"error": "..."} on failure; we must check for this
// before attempting to unmarshal into the expected result type.
type hostErrorResponse struct {
	Error string `json:"error"`
}

func checkHostError(outBytes []byte) error {
	var errResp hostErrorResponse
	if err := json.Unmarshal(outBytes, &errResp); err == nil && errResp.Error != "" {
		return fmt.Errorf("%s", errResp.Error)
	}
	return nil
}

func callDBQuery(sql string, args []any) (dbQueryOutput, error) {
	input := dbQueryInput{SQL: sql, Args: args}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return dbQueryOutput{}, fmt.Errorf("marshal query input: %w", err)
	}
	mem := pdk.AllocateBytes(inputBytes)
	defer mem.Free()

	outOffset := hostDBQuery(mem.Offset())
	outMem := pdk.FindMemory(outOffset)
	outBytes := outMem.ReadBytes()

	if err := checkHostError(outBytes); err != nil {
		return dbQueryOutput{}, err
	}

	var out dbQueryOutput
	if err := json.Unmarshal(outBytes, &out); err != nil {
		return dbQueryOutput{}, fmt.Errorf("unmarshal query output: %w", err)
	}
	return out, nil
}

func callDBExec(sql string, args []any) (dbExecOutput, error) {
	input := dbExecInput{SQL: sql, Args: args}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return dbExecOutput{}, fmt.Errorf("marshal exec input: %w", err)
	}
	mem := pdk.AllocateBytes(inputBytes)
	defer mem.Free()

	outOffset := hostDBExec(mem.Offset())
	outMem := pdk.FindMemory(outOffset)
	outBytes := outMem.ReadBytes()

	if err := checkHostError(outBytes); err != nil {
		return dbExecOutput{}, err
	}

	var out dbExecOutput
	if err := json.Unmarshal(outBytes, &out); err != nil {
		return dbExecOutput{}, fmt.Errorf("unmarshal exec output: %w", err)
	}
	return out, nil
}

func emitEvent(eventName string, payload any) {
	input := emitInput{EventName: eventName, Payload: payload}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return
	}
	mem := pdk.AllocateBytes(inputBytes)
	defer mem.Free()
	hostEventEmit(mem.Offset()) //nolint:errcheck — best-effort
}

func nowUTC() string {
	mem := pdk.AllocateBytes([]byte("{}"))
	defer mem.Free()
	outOffset := hostTimeNow(mem.Offset())
	outMem := pdk.FindMemory(outOffset)
	outBytes := outMem.ReadBytes()
	var result struct {
		Now string `json:"now"`
	}
	if err := json.Unmarshal(outBytes, &result); err != nil {
		return "1970-01-01T00:00:00Z"
	}
	return result.Now
}

// ---- One-time migration: drop legacy global unique indexes ----

var legacyIndexesDropped bool

func dropLegacyIndexes() {
	if legacyIndexesDropped {
		return
	}
	legacyIndexesDropped = true
	callDBExec("DROP INDEX IF EXISTS idx_board_columns_name", nil)
	callDBExec("DROP INDEX IF EXISTS idx_task_types_name", nil)
}

// ---- Seed defaults ----

func ensureDefaultBoardColumns(projectID *int64) {
	dropLegacyIndexes()
	var out dbQueryOutput
	var err error
	if projectID != nil {
		out, err = callDBQuery("SELECT COUNT(*) as cnt FROM board_columns WHERE project_id = ?1", []any{*projectID})
	} else {
		out, err = callDBQuery("SELECT COUNT(*) as cnt FROM board_columns WHERE project_id IS NULL", nil)
	}
	if err != nil || len(out.Rows) == 0 {
		// Table might not exist yet — try inserting anyway.
		insertDefaultBoardColumns(projectID)
		return
	}
	ci := makeColIdx(out.Columns)
	if getInt64(ci, out.Rows[0], "cnt") > 0 {
		return
	}
	insertDefaultBoardColumns(projectID)
}

func insertDefaultBoardColumns(projectID *int64) {
	now := nowUTC()
	var pidVal any
	if projectID != nil {
		pidVal = *projectID
	}
	defaults := []struct {
		name  string
		label string
		pos   int
	}{
		{"pending", "Pending", 0},
		{"in_progress", "In Progress", 1},
		{"done", "Done", 2},
	}
	for _, d := range defaults {
		_, err := callDBExec(
			"INSERT OR IGNORE INTO board_columns (name, label, position, is_default, project_id, created_at) VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
			[]any{d.name, d.label, d.pos, 1, pidVal, now},
		)
		_ = err // INSERT OR IGNORE handles duplicates; other errors are non-fatal.
	}
}

func ensureDefaultTaskTypes(projectID *int64) {
	var out dbQueryOutput
	var err error
	if projectID != nil {
		out, err = callDBQuery("SELECT COUNT(*) as cnt FROM task_types WHERE project_id = ?1", []any{*projectID})
	} else {
		out, err = callDBQuery("SELECT COUNT(*) as cnt FROM task_types WHERE project_id IS NULL", nil)
	}
	if err != nil || len(out.Rows) == 0 {
		return
	}
	ci := makeColIdx(out.Columns)
	if getInt64(ci, out.Rows[0], "cnt") > 0 {
		return
	}
	now := nowUTC()
	var pidVal any
	if projectID != nil {
		pidVal = *projectID
	}
	callDBExec("INSERT INTO task_types (name, label, project_id, created_at) VALUES (?1, ?2, ?3, ?4)", []any{"task", "Task", pidVal, now})
	callDBExec("INSERT INTO task_types (name, label, color, icon, project_id, created_at) VALUES (?1, ?2, ?3, ?4, ?5, ?6)", []any{"feature", "Feature", "#22c55e", "star", pidVal, now})
	callDBExec("INSERT INTO task_types (name, label, color, icon, project_id, created_at) VALUES (?1, ?2, ?3, ?4, ?5, ?6)", []any{"bug_fix", "Bug Fix", "#ef4444", "zap", pidVal, now})
}

// loadValidStatuses fetches board column names for status validation.
// When projectID is non-nil it loads statuses for that project; otherwise global (NULL project_id).
func loadValidStatuses(projectID *int64) ([]string, error) {
	var out dbQueryOutput
	var err error
	if projectID != nil {
		out, err = callDBQuery("SELECT name FROM board_columns WHERE project_id = ?1 ORDER BY position ASC", []any{*projectID})
	} else {
		out, err = callDBQuery("SELECT name FROM board_columns WHERE project_id IS NULL ORDER BY position ASC", nil)
	}
	if err != nil {
		return nil, err
	}
	ci := makeColIdx(out.Columns)
	statuses := make([]string, 0, len(out.Rows))
	for _, row := range out.Rows {
		statuses = append(statuses, getStr(ci, row, "name"))
	}
	return statuses, nil
}

// loadBoardColumns fetches board columns ordered by position.
// When projectID is non-nil it loads columns for that project; otherwise global (NULL project_id).
func loadBoardColumns(projectID *int64) ([]BoardColumnDef, error) {
	var out dbQueryOutput
	var err error
	if projectID != nil {
		out, err = callDBQuery("SELECT id, name, label, position, color, project_id, is_default, created_at FROM board_columns WHERE project_id = ?1 ORDER BY position ASC", []any{*projectID})
	} else {
		out, err = callDBQuery("SELECT id, name, label, position, color, project_id, is_default, created_at FROM board_columns WHERE project_id IS NULL ORDER BY position ASC", nil)
	}
	if err != nil {
		return nil, err
	}
	cols := make([]BoardColumnDef, 0, len(out.Rows))
	for _, row := range out.Rows {
		c, err := rowToBoardColumnDef(out.Columns, row)
		if err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, nil
}

// loadTaskTypes fetches task types.
// When projectID is non-nil it loads types for that project; otherwise global (NULL project_id).
func loadTaskTypes(projectID *int64) ([]TaskType, error) {
	var out dbQueryOutput
	var err error
	if projectID != nil {
		out, err = callDBQuery("SELECT id, name, label, color, icon, project_id, created_at FROM task_types WHERE project_id = ?1 ORDER BY id ASC", []any{*projectID})
	} else {
		out, err = callDBQuery("SELECT id, name, label, color, icon, project_id, created_at FROM task_types WHERE project_id IS NULL ORDER BY id ASC", nil)
	}
	if err != nil {
		return nil, err
	}
	types := make([]TaskType, 0, len(out.Rows))
	for _, row := range out.Rows {
		t, err := rowToTaskType(out.Columns, row)
		if err != nil {
			return nil, err
		}
		types = append(types, t)
	}
	return types, nil
}

// ---- Task SELECT constant ----

const taskSelectSQL = "SELECT id, title, description, status, priority, due_date, project_id, type_id, assignee_id, created_at, updated_at FROM tasks"

// ---- Exported tool functions ----

//go:wasmexport list_tasks
func listTasks() int32 {
	var params ListTasksParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}

	if params.StatusFilter != "" {
		ensureDefaultBoardColumns(params.ProjectID)
		statuses, err := loadValidStatuses(params.ProjectID)
		if err != nil {
			pdk.OutputString(fmt.Sprintf("load statuses: %s", err))
			return 1
		}
		if err := validateStatus(params.StatusFilter, statuses); err != nil {
			pdk.OutputString(err.Error())
			return 1
		}
	}

	sql, args := buildListTasksQuery(params)

	out, err := callDBQuery(sql, args)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db query error: %s", err))
		return 1
	}

	tasks := make([]Task, 0, len(out.Rows))
	for _, row := range out.Rows {
		t, rowErr := rowToTask(out.Columns, row)
		if rowErr != nil {
			pdk.OutputString(fmt.Sprintf("row scan error: %s", rowErr))
			return 1
		}
		tasks = append(tasks, t)
	}

	result := ListTasksResult{Tasks: tasks, Total: len(tasks)}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport create_task
func createTask() int32 {
	var params CreateTaskParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}

	if params.Title == "" {
		pdk.OutputString("title is required")
		return 1
	}
	if params.Status == "" {
		params.Status = "pending"
	}

	ensureDefaultBoardColumns(params.ProjectID)
	statuses, err := loadValidStatuses(params.ProjectID)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("load statuses: %s", err))
		return 1
	}
	if err := validateStatus(params.Status, statuses); err != nil {
		pdk.OutputString(err.Error())
		return 1
	}
	if params.Priority == 0 {
		params.Priority = 3
	}

	now := nowUTC()

	var projectIDVal, typeIDVal, assigneeIDVal any
	if params.ProjectID != nil {
		projectIDVal = *params.ProjectID
	}
	if params.TypeID != nil {
		typeIDVal = *params.TypeID
	}
	if params.AssigneeID != nil {
		assigneeIDVal = *params.AssigneeID
	}

	execOut, err := callDBExec(
		"INSERT INTO tasks (title, description, status, priority, due_date, project_id, type_id, assignee_id, created_at, updated_at) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)",
		[]any{params.Title, params.Description, params.Status, params.Priority, params.DueDate, projectIDVal, typeIDVal, assigneeIDVal, now, now},
	)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db exec error: %s", err))
		return 1
	}

	task := Task{
		ID:          execOut.LastInsertID,
		Title:       params.Title,
		Description: params.Description,
		Status:      params.Status,
		Priority:    params.Priority,
		DueDate:     params.DueDate,
		ProjectID:   params.ProjectID,
		TypeID:      params.TypeID,
		AssigneeID:  params.AssigneeID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	emitEvent("task.created", map[string]any{
		"task_id":    task.ID,
		"title":      task.Title,
		"status":     task.Status,
		"priority":   task.Priority,
		"created_at": task.CreatedAt,
	})

	taskURL := fmt.Sprintf("/p/tasks-plugin/board?task=%d", task.ID)
	if task.ProjectID != nil {
		taskURL = fmt.Sprintf("/p/projects-plugin/project/%d/tasks?task=%d", *task.ProjectID, task.ID)
	}
	resultBytes, err := json.Marshal(CreateTaskResult{Task: task, WebURL: taskURL})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport get_task
func getTask() int32 {
	var params GetTaskParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}

	if params.ID == 0 {
		pdk.OutputString("task id is required")
		return 1
	}

	out, err := callDBQuery(taskSelectSQL+" WHERE id = ?1", []any{params.ID})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db query error: %s", err))
		return 1
	}
	if len(out.Rows) == 0 {
		pdk.OutputString("task not found")
		return 1
	}

	task, err := rowToTask(out.Columns, out.Rows[0])
	if err != nil {
		pdk.OutputString(fmt.Sprintf("row scan error: %s", err))
		return 1
	}

	resultBytes, err := json.Marshal(GetTaskResult{Task: task})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport update_task
func updateTask() int32 {
	var params UpdateTaskParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}

	if params.ID == 0 {
		pdk.OutputString("id is required")
		return 1
	}

	currentOut, err := callDBQuery(taskSelectSQL+" WHERE id = ?1", []any{params.ID})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db query error: %s", err))
		return 1
	}
	if len(currentOut.Rows) == 0 {
		pdk.OutputString("task not found")
		return 1
	}
	current, err := rowToTask(currentOut.Columns, currentOut.Rows[0])
	if err != nil {
		pdk.OutputString(fmt.Sprintf("row scan error: %s", err))
		return 1
	}

	if params.Status != "" {
		ensureDefaultBoardColumns(current.ProjectID)
		statuses, sErr := loadValidStatuses(current.ProjectID)
		if sErr != nil {
			pdk.OutputString(fmt.Sprintf("load statuses: %s", sErr))
			return 1
		}
		if err := validateStatus(params.Status, statuses); err != nil {
			pdk.OutputString(err.Error())
			return 1
		}
	}

	setClauses := []string{}
	args := []any{}

	if params.Title != "" {
		args = append(args, params.Title)
		setClauses = append(setClauses, fmt.Sprintf("title = ?%d", len(args)))
	}
	if params.ClearDescription {
		args = append(args, nil)
		setClauses = append(setClauses, fmt.Sprintf("description = ?%d", len(args)))
	} else if params.Description != "" {
		args = append(args, params.Description)
		setClauses = append(setClauses, fmt.Sprintf("description = ?%d", len(args)))
	}
	if params.Status != "" {
		args = append(args, params.Status)
		setClauses = append(setClauses, fmt.Sprintf("status = ?%d", len(args)))
	}
	if params.Priority != 0 {
		args = append(args, params.Priority)
		setClauses = append(setClauses, fmt.Sprintf("priority = ?%d", len(args)))
	}
	if params.ClearDueDate {
		args = append(args, nil)
		setClauses = append(setClauses, fmt.Sprintf("due_date = ?%d", len(args)))
	} else if params.DueDate != "" {
		args = append(args, params.DueDate)
		setClauses = append(setClauses, fmt.Sprintf("due_date = ?%d", len(args)))
	}
	if params.ClearType {
		args = append(args, nil)
		setClauses = append(setClauses, fmt.Sprintf("type_id = ?%d", len(args)))
	} else if params.TypeID != nil {
		args = append(args, *params.TypeID)
		setClauses = append(setClauses, fmt.Sprintf("type_id = ?%d", len(args)))
	}
	if params.ClearAssignee {
		args = append(args, nil)
		setClauses = append(setClauses, fmt.Sprintf("assignee_id = ?%d", len(args)))
	} else if params.AssigneeID != nil {
		args = append(args, *params.AssigneeID)
		setClauses = append(setClauses, fmt.Sprintf("assignee_id = ?%d", len(args)))
	}

	now := nowUTC()
	args = append(args, now)
	setClauses = append(setClauses, fmt.Sprintf("updated_at = ?%d", len(args)))

	sql := "UPDATE tasks SET "
	for i, clause := range setClauses {
		if i > 0 {
			sql += ", "
		}
		sql += clause
	}
	args = append(args, params.ID)
	sql += fmt.Sprintf(" WHERE id = ?%d", len(args))

	execOut, err := callDBExec(sql, args)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db exec error: %s", err))
		return 1
	}
	if execOut.RowsAffected == 0 {
		pdk.OutputString("task not found")
		return 1
	}

	updatedOut, err := callDBQuery(taskSelectSQL+" WHERE id = ?1", []any{params.ID})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db query error: %s", err))
		return 1
	}
	if len(updatedOut.Rows) == 0 {
		pdk.OutputString("task not found after update")
		return 1
	}
	updated, err := rowToTask(updatedOut.Columns, updatedOut.Rows[0])
	if err != nil {
		pdk.OutputString(fmt.Sprintf("row scan error: %s", err))
		return 1
	}

	emitEvent("task.updated", map[string]any{
		"task_id":         updated.ID,
		"previous_status": current.Status,
		"new_status":      updated.Status,
		"updated_at":      updated.UpdatedAt,
	})

	if current.Status != "done" && updated.Status == "done" {
		emitEvent("task.completed", map[string]any{
			"task_id":      updated.ID,
			"title":        updated.Title,
			"completed_at": updated.UpdatedAt,
		})
	}

	resultBytes, err := json.Marshal(UpdateTaskResult{Task: updated})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport delete_task
func deleteTask() int32 {
	var params DeleteTaskParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}

	// Cascade: delete comments first.
	callDBExec("DELETE FROM comments WHERE task_id = ?1", []any{params.ID})

	execOut, err := callDBExec("DELETE FROM tasks WHERE id = ?1", []any{params.ID})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db exec error: %s", err))
		return 1
	}

	deleted := execOut.RowsAffected > 0
	if deleted {
		emitEvent("task.deleted", map[string]any{
			"task_id":    params.ID,
			"deleted_at": nowUTC(),
		})
	}

	resultBytes, err := json.Marshal(DeleteTaskResult{Deleted: deleted, ID: params.ID})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

// ---- Board column CRUD ----

//go:wasmexport list_board_columns
func listBoardColumns() int32 {
	var input struct {
		ProjectID *int64 `json:"project_id,omitempty"`
	}
	_ = json.Unmarshal([]byte(pdk.InputString()), &input)

	ensureDefaultBoardColumns(input.ProjectID)
	cols, err := loadBoardColumns(input.ProjectID)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db query error: %s", err))
		return 1
	}
	resultBytes, err := json.Marshal(ListBoardColumnsResult{Columns: cols})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport create_board_column
func createBoardColumn() int32 {
	var params CreateBoardColumnParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}
	if params.Name == "" {
		pdk.OutputString("name is required")
		return 1
	}
	if params.Label == "" {
		params.Label = params.Name
	}

	// Auto-position at end if not specified.
	if params.Position == 0 {
		var posOut dbQueryOutput
		var posErr error
		if params.ProjectID != nil {
			posOut, posErr = callDBQuery("SELECT COALESCE(MAX(position), -1) + 1 as next_pos FROM board_columns WHERE project_id = ?1", []any{*params.ProjectID})
		} else {
			posOut, posErr = callDBQuery("SELECT COALESCE(MAX(position), -1) + 1 as next_pos FROM board_columns WHERE project_id IS NULL", nil)
		}
		if posErr == nil && len(posOut.Rows) > 0 {
			ci := makeColIdx(posOut.Columns)
			params.Position = int(getInt64(ci, posOut.Rows[0], "next_pos"))
		}
	}

	var pidVal any
	if params.ProjectID != nil {
		pidVal = *params.ProjectID
	}

	now := nowUTC()
	execOut, err := callDBExec(
		"INSERT INTO board_columns (name, label, position, color, is_default, project_id, created_at) VALUES (?1, ?2, ?3, ?4, 0, ?5, ?6)",
		[]any{params.Name, params.Label, params.Position, params.Color, pidVal, now},
	)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db exec error: %s", err))
		return 1
	}

	col := BoardColumnDef{
		ID:        execOut.LastInsertID,
		Name:      params.Name,
		Label:     params.Label,
		Position:  params.Position,
		Color:     params.Color,
		ProjectID: params.ProjectID,
		IsDefault: false,
		CreatedAt: now,
	}
	resultBytes, err := json.Marshal(CreateBoardColumnResult{Column: col})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport update_board_column
func updateBoardColumn() int32 {
	var params UpdateBoardColumnParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}
	if params.ID == 0 {
		pdk.OutputString("id is required")
		return 1
	}

	setClauses := []string{}
	args := []any{}

	if params.Label != "" {
		args = append(args, params.Label)
		setClauses = append(setClauses, fmt.Sprintf("label = ?%d", len(args)))
	}
	if params.Position != nil {
		args = append(args, *params.Position)
		setClauses = append(setClauses, fmt.Sprintf("position = ?%d", len(args)))
	}
	if params.Color != "" {
		args = append(args, params.Color)
		setClauses = append(setClauses, fmt.Sprintf("color = ?%d", len(args)))
	}

	if len(setClauses) == 0 {
		pdk.OutputString("no fields to update")
		return 1
	}

	sql := "UPDATE board_columns SET "
	for i, clause := range setClauses {
		if i > 0 {
			sql += ", "
		}
		sql += clause
	}
	args = append(args, params.ID)
	sql += fmt.Sprintf(" WHERE id = ?%d", len(args))

	execOut, err := callDBExec(sql, args)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db exec error: %s", err))
		return 1
	}
	if execOut.RowsAffected == 0 {
		pdk.OutputString("board column not found")
		return 1
	}

	// Reload.
	out, err := callDBQuery("SELECT id, name, label, position, color, project_id, is_default, created_at FROM board_columns WHERE id = ?1", []any{params.ID})
	if err != nil || len(out.Rows) == 0 {
		pdk.OutputString("board column not found after update")
		return 1
	}
	col, _ := rowToBoardColumnDef(out.Columns, out.Rows[0])
	resultBytes, err := json.Marshal(UpdateBoardColumnResult{Column: col})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport delete_board_column
func deleteBoardColumn() int32 {
	var params DeleteBoardColumnParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}

	// Check if it's a default column — cannot delete defaults.
	out, err := callDBQuery("SELECT is_default, name FROM board_columns WHERE id = ?1", []any{params.ID})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db query error: %s", err))
		return 1
	}
	if len(out.Rows) == 0 {
		pdk.OutputString("board column not found")
		return 1
	}
	ci := makeColIdx(out.Columns)
	if getBool(ci, out.Rows[0], "is_default") {
		pdk.OutputString("cannot delete a default board column")
		return 1
	}

	// Check if any tasks use this status.
	colName := getStr(ci, out.Rows[0], "name")
	taskCheck, err := callDBQuery("SELECT COUNT(*) as cnt FROM tasks WHERE status = ?1", []any{colName})
	if err == nil && len(taskCheck.Rows) > 0 {
		tci := makeColIdx(taskCheck.Columns)
		if getInt64(tci, taskCheck.Rows[0], "cnt") > 0 {
			pdk.OutputString(fmt.Sprintf("cannot delete column: %d tasks still have status %q", getInt64(tci, taskCheck.Rows[0], "cnt"), colName))
			return 1
		}
	}

	execOut, err := callDBExec("DELETE FROM board_columns WHERE id = ?1", []any{params.ID})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db exec error: %s", err))
		return 1
	}

	resultBytes, _ := json.Marshal(DeleteBoardColumnResult{Deleted: execOut.RowsAffected > 0, ID: params.ID})
	pdk.OutputString(string(resultBytes))
	return 0
}

// ---- Task type CRUD ----

//go:wasmexport list_task_types
func listTaskTypes() int32 {
	var input struct {
		ProjectID *int64 `json:"project_id,omitempty"`
	}
	_ = json.Unmarshal([]byte(pdk.InputString()), &input)

	ensureDefaultTaskTypes(input.ProjectID)
	types, err := loadTaskTypes(input.ProjectID)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db query error: %s", err))
		return 1
	}
	resultBytes, err := json.Marshal(ListTaskTypesResult{Types: types})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport create_task_type
func createTaskType() int32 {
	var params CreateTaskTypeParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}
	if params.Name == "" {
		pdk.OutputString("name is required")
		return 1
	}
	if params.Label == "" {
		params.Label = params.Name
	}

	var pidVal any
	if params.ProjectID != nil {
		pidVal = *params.ProjectID
	}

	now := nowUTC()
	execOut, err := callDBExec(
		"INSERT INTO task_types (name, label, color, icon, project_id, created_at) VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
		[]any{params.Name, params.Label, params.Color, params.Icon, pidVal, now},
	)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db exec error: %s", err))
		return 1
	}

	tt := TaskType{
		ID:        execOut.LastInsertID,
		Name:      params.Name,
		Label:     params.Label,
		Color:     params.Color,
		Icon:      params.Icon,
		ProjectID: params.ProjectID,
		CreatedAt: now,
	}
	resultBytes, err := json.Marshal(CreateTaskTypeResult{Type: tt})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport update_task_type
func updateTaskType() int32 {
	var params UpdateTaskTypeParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}
	if params.ID == 0 {
		pdk.OutputString("id is required")
		return 1
	}

	setClauses := []string{}
	args := []any{}

	if params.Label != "" {
		args = append(args, params.Label)
		setClauses = append(setClauses, fmt.Sprintf("label = ?%d", len(args)))
	}
	if params.Color != "" {
		args = append(args, params.Color)
		setClauses = append(setClauses, fmt.Sprintf("color = ?%d", len(args)))
	}
	if params.Icon != "" {
		args = append(args, params.Icon)
		setClauses = append(setClauses, fmt.Sprintf("icon = ?%d", len(args)))
	}

	if len(setClauses) == 0 {
		pdk.OutputString("no fields to update")
		return 1
	}

	sql := "UPDATE task_types SET "
	for i, clause := range setClauses {
		if i > 0 {
			sql += ", "
		}
		sql += clause
	}
	args = append(args, params.ID)
	sql += fmt.Sprintf(" WHERE id = ?%d", len(args))

	execOut, err := callDBExec(sql, args)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db exec error: %s", err))
		return 1
	}
	if execOut.RowsAffected == 0 {
		pdk.OutputString("task type not found")
		return 1
	}

	// Reload.
	out, err := callDBQuery("SELECT id, name, label, color, icon, project_id, created_at FROM task_types WHERE id = ?1", []any{params.ID})
	if err != nil || len(out.Rows) == 0 {
		pdk.OutputString("task type not found after update")
		return 1
	}
	tt, _ := rowToTaskType(out.Columns, out.Rows[0])
	resultBytes, _ := json.Marshal(UpdateTaskTypeResult{Type: tt})
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport delete_task_type
func deleteTaskType() int32 {
	var params DeleteTaskTypeParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}

	execOut, err := callDBExec("DELETE FROM task_types WHERE id = ?1", []any{params.ID})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db exec error: %s", err))
		return 1
	}

	// Nullify type_id on tasks that referenced this type.
	if execOut.RowsAffected > 0 {
		callDBExec("UPDATE tasks SET type_id = NULL WHERE type_id = ?1", []any{params.ID})
	}

	resultBytes, _ := json.Marshal(DeleteTaskTypeResult{Deleted: execOut.RowsAffected > 0, ID: params.ID})
	pdk.OutputString(string(resultBytes))
	return 0
}

// ---- Comment CRUD ----

//go:wasmexport list_comments
func listComments() int32 {
	var params ListCommentsParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}
	if params.TaskID == 0 {
		pdk.OutputString("task_id is required")
		return 1
	}

	out, err := callDBQuery(
		"SELECT id, task_id, author_type, author_id, author_name, body, created_at FROM comments WHERE task_id = ?1 ORDER BY created_at ASC",
		[]any{params.TaskID},
	)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db query error: %s", err))
		return 1
	}

	comments := make([]Comment, 0, len(out.Rows))
	for _, row := range out.Rows {
		c, rowErr := rowToComment(out.Columns, row)
		if rowErr != nil {
			pdk.OutputString(fmt.Sprintf("row scan error: %s", rowErr))
			return 1
		}
		comments = append(comments, c)
	}

	resultBytes, err := json.Marshal(ListCommentsResult{Comments: comments, Total: len(comments)})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport create_comment
func createComment() int32 {
	var params CreateCommentParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}
	if params.TaskID == 0 {
		pdk.OutputString("task_id is required")
		return 1
	}
	if params.Body == "" {
		pdk.OutputString("body is required")
		return 1
	}
	if params.AuthorName == "" && params.McAuthor != "" {
		params.AuthorName = params.McAuthor
		params.AuthorType = "agent"
	}
	if params.AuthorName == "" {
		params.AuthorName = "Anonymous"
	}
	if params.AuthorType == "" {
		params.AuthorType = "user"
	}

	var authorIDVal any
	if params.AuthorID != nil {
		authorIDVal = *params.AuthorID
	}

	now := nowUTC()
	execOut, err := callDBExec(
		"INSERT INTO comments (task_id, author_type, author_id, author_name, body, created_at) VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
		[]any{params.TaskID, params.AuthorType, authorIDVal, params.AuthorName, params.Body, now},
	)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db exec error: %s", err))
		return 1
	}

	comment := Comment{
		ID:         execOut.LastInsertID,
		TaskID:     params.TaskID,
		AuthorType: params.AuthorType,
		AuthorID:   params.AuthorID,
		AuthorName: params.AuthorName,
		Body:       params.Body,
		CreatedAt:  now,
	}

	emitEvent("comment.created", map[string]any{
		"comment_id":  comment.ID,
		"task_id":     comment.TaskID,
		"author_name": comment.AuthorName,
		"author_type": comment.AuthorType,
		"created_at":  comment.CreatedAt,
	})

	resultBytes, err := json.Marshal(CreateCommentResult{Comment: comment})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport delete_comment
func deleteComment() int32 {
	var params DeleteCommentParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}

	execOut, err := callDBExec("DELETE FROM comments WHERE id = ?1", []any{params.ID})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db exec error: %s", err))
		return 1
	}

	if execOut.RowsAffected > 0 {
		emitEvent("comment.deleted", map[string]any{
			"comment_id": params.ID,
			"deleted_at": nowUTC(),
		})
	}

	resultBytes, _ := json.Marshal(DeleteCommentResult{Deleted: execOut.RowsAffected > 0, ID: params.ID})
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport update_comment
func updateComment() int32 {
	var params UpdateCommentParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}
	if params.ID == 0 || params.Body == "" {
		pdk.OutputString("id and body required")
		return 1
	}

	execOut, err := callDBExec("UPDATE comments SET body = ?1 WHERE id = ?2", []any{params.Body, params.ID})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db exec error: %s", err))
		return 1
	}

	resultBytes, _ := json.Marshal(map[string]any{"updated": execOut.RowsAffected > 0, "id": params.ID})
	pdk.OutputString(string(resultBytes))
	return 0
}

// ---- Task detail (enriched) ----

//go:wasmexport get_task_detail
func getTaskDetail() int32 {
	var params GetTaskParams
	if err := json.Unmarshal([]byte(pdk.InputString()), &params); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}
	if params.ID == 0 {
		pdk.OutputString("task id is required")
		return 1
	}

	// Load task.
	out, err := callDBQuery(taskSelectSQL+" WHERE id = ?1", []any{params.ID})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db query error: %s", err))
		return 1
	}
	if len(out.Rows) == 0 {
		pdk.OutputString("task not found")
		return 1
	}
	task, err := rowToTask(out.Columns, out.Rows[0])
	if err != nil {
		pdk.OutputString(fmt.Sprintf("row scan error: %s", err))
		return 1
	}

	// Load comments.
	commOut, err := callDBQuery(
		"SELECT id, task_id, author_type, author_id, author_name, body, created_at FROM comments WHERE task_id = ?1 ORDER BY created_at ASC",
		[]any{params.ID},
	)
	comments := make([]Comment, 0)
	if err == nil {
		for _, row := range commOut.Rows {
			c, _ := rowToComment(commOut.Columns, row)
			comments = append(comments, c)
		}
	}

	// Load type info if task has a type_id.
	var typeInfo *TaskType
	if task.TypeID != nil {
		ttOut, err := callDBQuery("SELECT id, name, label, color, icon, project_id, created_at FROM task_types WHERE id = ?1", []any{*task.TypeID})
		if err == nil && len(ttOut.Rows) > 0 {
			tt, _ := rowToTaskType(ttOut.Columns, ttOut.Rows[0])
			typeInfo = &tt
		}
	}

	// Load board columns for status dropdown in detail view, scoped to task's project.
	ensureDefaultBoardColumns(task.ProjectID)
	cols, _ := loadBoardColumns(task.ProjectID)

	// Load all task types for type selector in detail view.
	ensureDefaultTaskTypes(task.ProjectID)
	taskTypes, _ := loadTaskTypes(task.ProjectID)

	result := TaskDetailResult{
		Task:      task,
		Comments:  comments,
		TypeInfo:  typeInfo,
		Columns:   cols,
		TaskTypes: taskTypes,
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

// ---- Dashboard widget functions ----

//go:wasmexport get_task_overview
func getTaskOverview() int32 {
	out, err := callDBQuery(
		"SELECT id, title, status, priority, project_id, created_at FROM tasks ORDER BY created_at DESC LIMIT 5",
		nil,
	)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db query error: %s", err))
		return 1
	}

	items := make([]TaskOverviewItem, 0, len(out.Rows))
	for _, row := range out.Rows {
		item, rowErr := rowToTaskOverviewItem(out.Columns, row)
		if rowErr != nil {
			pdk.OutputString(fmt.Sprintf("row scan error: %s", rowErr))
			return 1
		}
		items = append(items, item)
	}

	result := TaskOverviewResult{Tasks: items, Total: len(items)}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

// ---- Plugin page functions ----

//go:wasmexport get_tasks_board
func getTasksBoard() int32 {
	// Load all tasks, optionally filtered by project_id from input.
	var input struct {
		ProjectID *int64 `json:"project_id,omitempty"`
	}
	_ = json.Unmarshal([]byte(pdk.InputString()), &input)

	ensureDefaultBoardColumns(input.ProjectID)
	ensureDefaultTaskTypes(input.ProjectID)

	// Load dynamic board columns scoped to project.
	boardCols, err := loadBoardColumns(input.ProjectID)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("load columns: %s", err))
		return 1
	}

	// Load task types for badge rendering, scoped to project.
	taskTypes, _ := loadTaskTypes(input.ProjectID)

	query := taskSelectSQL
	var args []any
	if input.ProjectID != nil {
		query += " WHERE project_id = ?1"
		args = append(args, *input.ProjectID)
	}
	query += " ORDER BY priority ASC, created_at DESC"
	out, err := callDBQuery(query, args)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db query error: %s", err))
		return 1
	}

	tasks := make([]Task, 0, len(out.Rows))
	for _, row := range out.Rows {
		t, rowErr := rowToTask(out.Columns, row)
		if rowErr != nil {
			pdk.OutputString(fmt.Sprintf("row scan error: %s", rowErr))
			return 1
		}
		tasks = append(tasks, t)
	}

	// Group tasks by status into dynamic columns.
	grouped := map[string][]Task{}
	for _, t := range tasks {
		grouped[t.Status] = append(grouped[t.Status], t)
	}

	columns := make([]BoardColumn, 0, len(boardCols))
	for _, bc := range boardCols {
		items := grouped[bc.Name]
		if items == nil {
			items = []Task{}
		}
		columns = append(columns, BoardColumn{
			ID:        bc.Name,
			DbID:      bc.ID,
			Title:     bc.Label,
			Color:     bc.Color,
			IsDefault: bc.IsDefault,
			Position:  bc.Position,
			Items:     items,
		})
	}

	resultBytes, err := json.Marshal(BoardResult{Columns: columns, TaskTypes: taskTypes})
	if err != nil {
		pdk.OutputString(fmt.Sprintf("marshal result: %s", err))
		return 1
	}
	pdk.OutputString(string(resultBytes))
	return 0
}

//go:wasmexport on_project_deleted
func onProjectDeleted() int32 {
	var payload struct {
		ProjectID int64 `json:"project_id"`
	}
	if err := json.Unmarshal([]byte(pdk.InputString()), &payload); err != nil {
		pdk.OutputString(fmt.Sprintf("invalid input: %s", err))
		return 1
	}

	now := nowUTC()
	_, err := callDBExec(
		"UPDATE tasks SET project_id = NULL, updated_at = ?1 WHERE project_id = ?2",
		[]any{now, payload.ProjectID},
	)
	if err != nil {
		pdk.OutputString(fmt.Sprintf("db exec error: %s", err))
		return 1
	}

	// Clean up project-scoped board columns and task types.
	callDBExec("DELETE FROM board_columns WHERE project_id = ?1", []any{payload.ProjectID})
	callDBExec("DELETE FROM task_types WHERE project_id = ?1", []any{payload.ProjectID})

	pdk.OutputString("{}")
	return 0
}

func main() {}
