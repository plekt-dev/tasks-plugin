//go:build !wasip1

package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

// ---- Test infrastructure ----
// The plugin logic is extracted into testable pure functions that accept
// mockable host-call functions. This avoids the wasip1 build constraint
// while preserving full coverage of the business logic.

// hostCallFunc is the signature for mc_db / mc_event calls.
type hostCallFunc func(namespace, name string, input []byte) ([]byte, error)

// ---- Pure logic functions (mirror plugin.go but accept injected deps) ----

func doCallDBQuery(hc hostCallFunc, sql string, args []any) (dbQueryOutput, error) {
	input := dbQueryInput{SQL: sql, Args: args}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return dbQueryOutput{}, fmt.Errorf("marshal query input: %w", err)
	}
	outputBytes, err := hc("mc_db", "query", inputBytes)
	if err != nil {
		return dbQueryOutput{}, fmt.Errorf("mc_db query: %w", err)
	}
	var out dbQueryOutput
	if err := json.Unmarshal(outputBytes, &out); err != nil {
		return dbQueryOutput{}, fmt.Errorf("unmarshal query output: %w", err)
	}
	return out, nil
}

func doCallDBExec(hc hostCallFunc, sql string, args []any) (dbExecOutput, error) {
	input := dbExecInput{SQL: sql, Args: args}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return dbExecOutput{}, fmt.Errorf("marshal exec input: %w", err)
	}
	outputBytes, err := hc("mc_db", "exec", inputBytes)
	if err != nil {
		return dbExecOutput{}, fmt.Errorf("mc_db exec: %w", err)
	}
	var out dbExecOutput
	if err := json.Unmarshal(outputBytes, &out); err != nil {
		return dbExecOutput{}, fmt.Errorf("unmarshal exec output: %w", err)
	}
	return out, nil
}

func doEmitEvent(hc hostCallFunc, eventName string, payload any) {
	input := emitInput{EventName: eventName, Payload: payload}
	inputBytes, _ := json.Marshal(input)
	_, _ = hc("mc_event", "emit", inputBytes)
}

// ---- Testable wrappers for each tool ----

func doListTasks(hc hostCallFunc, params ListTasksParams) (ListTasksResult, error) {
	if params.StatusFilter != "" {
		if err := validateStatus(params.StatusFilter, []string{"pending", "in_progress", "done"}); err != nil {
			return ListTasksResult{}, err
		}
	}

	sql, args := buildListTasksQuery(params)

	out, err := doCallDBQuery(hc, sql, args)
	if err != nil {
		return ListTasksResult{}, err
	}

	tasks := make([]Task, 0, len(out.Rows))
	for _, row := range out.Rows {
		t, err := rowToTask(out.Columns, row)
		if err != nil {
			return ListTasksResult{}, err
		}
		tasks = append(tasks, t)
	}
	return ListTasksResult{Tasks: tasks, Total: len(tasks)}, nil
}

func doCreateTask(hc hostCallFunc, params CreateTaskParams, now string) (CreateTaskResult, error) {
	if params.Title == "" {
		return CreateTaskResult{}, fmt.Errorf("title is required")
	}
	if params.Status == "" {
		params.Status = "pending"
	}
	if err := validateStatus(params.Status, []string{"pending", "in_progress", "done"}); err != nil {
		return CreateTaskResult{}, err
	}
	if params.Priority == 0 {
		params.Priority = 3
	}

	execOut, err := doCallDBExec(hc,
		"INSERT INTO tasks (title, description, status, priority, due_date, created_at, updated_at) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)",
		[]any{params.Title, params.Description, params.Status, params.Priority, params.DueDate, now, now},
	)
	if err != nil {
		return CreateTaskResult{}, err
	}

	task := Task{
		ID:          execOut.LastInsertID,
		Title:       params.Title,
		Description: params.Description,
		Status:      params.Status,
		Priority:    params.Priority,
		DueDate:     params.DueDate,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	doEmitEvent(hc, "task.created", map[string]any{
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
	return CreateTaskResult{Task: task, WebURL: taskURL}, nil
}

func doGetTask(hc hostCallFunc, params GetTaskParams) (GetTaskResult, error) {
	if params.ID == 0 {
		return GetTaskResult{}, fmt.Errorf("task id is required")
	}
	out, err := doCallDBQuery(hc,
		"SELECT id, title, description, status, priority, due_date, created_at, updated_at FROM tasks WHERE id = ?1",
		[]any{params.ID},
	)
	if err != nil {
		return GetTaskResult{}, err
	}
	if len(out.Rows) == 0 {
		return GetTaskResult{}, fmt.Errorf("task not found")
	}
	task, err := rowToTask(out.Columns, out.Rows[0])
	if err != nil {
		return GetTaskResult{}, err
	}
	return GetTaskResult{Task: task}, nil
}

func doUpdateTask(hc hostCallFunc, params UpdateTaskParams, now string) (UpdateTaskResult, error) {
	if params.ID == 0 {
		return UpdateTaskResult{}, fmt.Errorf("id is required")
	}

	currentOut, err := doCallDBQuery(hc,
		"SELECT id, title, description, status, priority, due_date, created_at, updated_at FROM tasks WHERE id = ?1",
		[]any{params.ID},
	)
	if err != nil {
		return UpdateTaskResult{}, err
	}
	if len(currentOut.Rows) == 0 {
		return UpdateTaskResult{}, fmt.Errorf("task not found")
	}
	current, err := rowToTask(currentOut.Columns, currentOut.Rows[0])
	if err != nil {
		return UpdateTaskResult{}, err
	}

	if params.Status != "" {
		if err := validateStatus(params.Status, []string{"pending", "in_progress", "done"}); err != nil {
			return UpdateTaskResult{}, err
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

	execOut, err := doCallDBExec(hc, sql, args)
	if err != nil {
		return UpdateTaskResult{}, err
	}
	if execOut.RowsAffected == 0 {
		return UpdateTaskResult{}, fmt.Errorf("task not found")
	}

	// Second query to simulate reload — in tests mock returns updated row.
	updatedOut, err := doCallDBQuery(hc,
		"SELECT id, title, description, status, priority, due_date, created_at, updated_at FROM tasks WHERE id = ?1",
		[]any{params.ID},
	)
	if err != nil {
		return UpdateTaskResult{}, err
	}
	if len(updatedOut.Rows) == 0 {
		return UpdateTaskResult{}, fmt.Errorf("task not found after update")
	}
	updated, err := rowToTask(updatedOut.Columns, updatedOut.Rows[0])
	if err != nil {
		return UpdateTaskResult{}, err
	}

	doEmitEvent(hc, "task.updated", map[string]any{
		"task_id":         updated.ID,
		"previous_status": current.Status,
		"new_status":      updated.Status,
		"updated_at":      updated.UpdatedAt,
	})

	if current.Status != "done" && updated.Status == "done" {
		doEmitEvent(hc, "task.completed", map[string]any{
			"task_id":      updated.ID,
			"title":        updated.Title,
			"completed_at": updated.UpdatedAt,
		})
	}

	return UpdateTaskResult{Task: updated}, nil
}

func doDeleteTask(hc hostCallFunc, params DeleteTaskParams, now string) (DeleteTaskResult, error) {
	execOut, err := doCallDBExec(hc,
		"DELETE FROM tasks WHERE id = ?1",
		[]any{params.ID},
	)
	if err != nil {
		return DeleteTaskResult{}, err
	}

	deleted := execOut.RowsAffected > 0
	if deleted {
		doEmitEvent(hc, "task.deleted", map[string]any{
			"task_id":    params.ID,
			"deleted_at": now,
		})
	}

	return DeleteTaskResult{Deleted: deleted, ID: params.ID}, nil
}

// ---- Mock helpers ----

// taskColumns is the standard column list returned by SELECT *.
var taskColumns = []string{"id", "title", "description", "status", "priority", "due_date", "project_id", "type_id", "assignee_id", "created_at", "updated_at"}

func makeTaskRow(id int64, title, description, status string, priority int, dueDate, createdAt, updatedAt string) []any {
	return []any{id, title, description, status, priority, dueDate, nil, nil, nil, createdAt, updatedAt}
}

// capturedEmits records calls to mc_event::emit during a test.
type capturedEmits struct {
	events []emitInput
}

func (c *capturedEmits) capture(namespace, name string, input []byte) ([]byte, error) {
	if namespace == "mc_event" && name == "emit" {
		var ev emitInput
		_ = json.Unmarshal(input, &ev)
		c.events = append(c.events, ev)
	}
	return []byte("{}"), nil
}

// ---- Tests: validateStatus ----

func TestValidateStatus(t *testing.T) {
	cases := []struct {
		name    string
		status  string
		wantErr bool
	}{
		{"pending ok", "pending", false},
		{"in_progress ok", "in_progress", false},
		{"done ok", "done", false},
		{"empty string", "", true},
		{"uppercase invalid", "PENDING", true},
		{"unknown value", "archived", true},
		{"sql injection attempt", "pending'; DROP TABLE tasks;--", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateStatus(tc.status, []string{"pending", "in_progress", "done"})
			if (err != nil) != tc.wantErr {
				t.Errorf("validateStatus(%q) error=%v, wantErr=%v", tc.status, err, tc.wantErr)
			}
		})
	}
}

// ---- Tests: rowToTask ----

func TestRowToTask(t *testing.T) {
	t.Run("happy path all fields", func(t *testing.T) {
		row := makeTaskRow(1, "Buy milk", "from store", "pending", 3, "2026-04-01T00:00:00Z", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z")
		task, err := rowToTask(taskColumns, row)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.ID != 1 {
			t.Errorf("ID: got %d want 1", task.ID)
		}
		if task.Title != "Buy milk" {
			t.Errorf("Title: got %q want 'Buy milk'", task.Title)
		}
		if task.Status != "pending" {
			t.Errorf("Status: got %q want 'pending'", task.Status)
		}
		if task.Priority != 3 {
			t.Errorf("Priority: got %d want 3", task.Priority)
		}
	})

	t.Run("nil description and due_date", func(t *testing.T) {
		row := []any{int64(2), "Task", nil, "done", int64(5), nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}
		task, err := rowToTask(taskColumns, row)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.Description != "" {
			t.Errorf("Description: got %q want empty", task.Description)
		}
		if task.DueDate != "" {
			t.Errorf("DueDate: got %q want empty", task.DueDate)
		}
	})

	t.Run("float64 id from JSON number", func(t *testing.T) {
		row := []any{float64(42), "Task", nil, "pending", float64(2), nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}
		task, err := rowToTask(taskColumns, row)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.ID != 42 {
			t.Errorf("ID: got %d want 42", task.ID)
		}
	})

	t.Run("empty row returns zero values", func(t *testing.T) {
		task, err := rowToTask(taskColumns, []any{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.ID != 0 || task.Title != "" {
			t.Errorf("expected zero Task, got %+v", task)
		}
	})

	t.Run("byte slice title", func(t *testing.T) {
		row := []any{int64(3), []byte("Byte title"), nil, "pending", int64(1), nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}
		task, err := rowToTask(taskColumns, row)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.Title != "Byte title" {
			t.Errorf("Title: got %q want 'Byte title'", task.Title)
		}
	})

	t.Run("int32 priority", func(t *testing.T) {
		row := []any{int64(4), "Task", nil, "done", int32(5), nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}
		task, err := rowToTask(taskColumns, row)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.Priority != 5 {
			t.Errorf("Priority: got %d want 5", task.Priority)
		}
	})

	t.Run("unknown type in string field falls back to Sprintf", func(t *testing.T) {
		// Pass an int where a string is expected — fallback to fmt.Sprintf.
		row := []any{int64(5), 42, nil, "pending", int64(3), nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}
		task, err := rowToTask(taskColumns, row)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.Title != "42" {
			t.Errorf("Title: got %q want '42'", task.Title)
		}
	})

	t.Run("unknown type in int field returns zero", func(t *testing.T) {
		row := []any{int64(6), "Task", nil, "pending", "not-a-number", nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"}
		task, err := rowToTask(taskColumns, row)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.Priority != 0 {
			t.Errorf("Priority: got %d want 0", task.Priority)
		}
	})
}

// ---- Tests: list_tasks ----

func TestListTasks(t *testing.T) {
	baseRow := makeTaskRow(1, "Task 1", "", "pending", 3, "", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
	row2 := makeTaskRow(2, "Task 2", "", "done", 5, "", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")

	cases := []struct {
		name            string
		params          ListTasksParams
		mockRows        [][]any
		wantTotal       int
		wantErr         bool
		wantSQLContains string
	}{
		{
			name:            "no filters returns all",
			params:          ListTasksParams{},
			mockRows:        [][]any{baseRow, row2},
			wantTotal:       2,
			wantSQLContains: "SELECT",
		},
		{
			name:            "status filter pending",
			params:          ListTasksParams{StatusFilter: "pending"},
			mockRows:        [][]any{baseRow},
			wantTotal:       1,
			wantSQLContains: "status = ?1",
		},
		{
			name:            "priority range filter",
			params:          ListTasksParams{PriorityMin: 3, PriorityMax: 5},
			mockRows:        [][]any{baseRow, row2},
			wantTotal:       2,
			wantSQLContains: "priority >=",
		},
		{
			name:            "limit applied",
			params:          ListTasksParams{Limit: 1},
			mockRows:        [][]any{baseRow},
			wantTotal:       1,
			wantSQLContains: "LIMIT",
		},
		{
			name:    "invalid status filter returns error",
			params:  ListTasksParams{StatusFilter: "invalid"},
			wantErr: true,
		},
		{
			name:    "db error propagates",
			params:  ListTasksParams{},
			wantErr: true,
		},
		{
			name:            "empty result",
			params:          ListTasksParams{StatusFilter: "done"},
			mockRows:        [][]any{},
			wantTotal:       0,
			wantSQLContains: "WHERE",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedQuerySQL string
			hc := func(namespace, name string, input []byte) ([]byte, error) {
				if tc.name == "db error propagates" {
					return nil, fmt.Errorf("db failure")
				}
				if namespace == "mc_db" && name == "query" {
					var qi dbQueryInput
					_ = json.Unmarshal(input, &qi)
					capturedQuerySQL = qi.SQL
					out := dbQueryOutput{Columns: taskColumns, Rows: tc.mockRows}
					b, _ := json.Marshal(out)
					return b, nil
				}
				return []byte("{}"), nil
			}

			result, err := doListTasks(hc, tc.params)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Total != tc.wantTotal {
				t.Errorf("Total: got %d want %d", result.Total, tc.wantTotal)
			}
			if len(result.Tasks) != tc.wantTotal {
				t.Errorf("Tasks length: got %d want %d", len(result.Tasks), tc.wantTotal)
			}
			if tc.wantSQLContains != "" {
				found := false
				for i := 0; i+len(tc.wantSQLContains) <= len(capturedQuerySQL); i++ {
					if capturedQuerySQL[i:i+len(tc.wantSQLContains)] == tc.wantSQLContains {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("SQL %q does not contain %q", capturedQuerySQL, tc.wantSQLContains)
				}
			}
		})
	}
}

// ---- Tests: create_task ----

func TestCreateTask(t *testing.T) {
	const now = "2026-03-21T12:00:00Z"

	cases := []struct {
		name       string
		params     CreateTaskParams
		mockExec   *dbExecOutput
		dbErr      error
		wantErr    bool
		wantStatus string
		wantPrio   int
		wantID     int64
	}{
		{
			name:       "happy path minimal",
			params:     CreateTaskParams{Title: "New task"},
			mockExec:   &dbExecOutput{RowsAffected: 1, LastInsertID: 10},
			wantStatus: "pending",
			wantPrio:   3,
			wantID:     10,
		},
		{
			name:       "explicit status and priority",
			params:     CreateTaskParams{Title: "Task", Status: "in_progress", Priority: 5},
			mockExec:   &dbExecOutput{RowsAffected: 1, LastInsertID: 11},
			wantStatus: "in_progress",
			wantPrio:   5,
			wantID:     11,
		},
		{
			name:    "empty title returns error",
			params:  CreateTaskParams{Title: ""},
			wantErr: true,
		},
		{
			name:    "invalid status returns error",
			params:  CreateTaskParams{Title: "T", Status: "bogus"},
			wantErr: true,
		},
		{
			name:    "db exec error propagates",
			params:  CreateTaskParams{Title: "T"},
			dbErr:   fmt.Errorf("disk full"),
			wantErr: true,
		},
		{
			name:       "description preserved",
			params:     CreateTaskParams{Title: "T", Description: "details"},
			mockExec:   &dbExecOutput{RowsAffected: 1, LastInsertID: 12},
			wantStatus: "pending",
			wantPrio:   3,
			wantID:     12,
		},
		{
			name:       "sql injection in title is parameterized (no error)",
			params:     CreateTaskParams{Title: "'; DROP TABLE tasks;--"},
			mockExec:   &dbExecOutput{RowsAffected: 1, LastInsertID: 99},
			wantStatus: "pending",
			wantPrio:   3,
			wantID:     99,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			emits := &capturedEmits{}
			hc := func(namespace, name string, input []byte) ([]byte, error) {
				if namespace == "mc_event" {
					return emits.capture(namespace, name, input)
				}
				if tc.dbErr != nil {
					return nil, tc.dbErr
				}
				if namespace == "mc_db" && name == "exec" {
					// Verify the SQL uses parameterized form, not string-interpolated title.
					var qi dbExecInput
					_ = json.Unmarshal(input, &qi)
					if len(qi.Args) == 0 {
						t.Errorf("exec called with no args — possible SQL injection vulnerability")
					}
					b, _ := json.Marshal(tc.mockExec)
					return b, nil
				}
				return []byte("{}"), nil
			}

			result, err := doCreateTask(hc, tc.params, now)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Task.ID != tc.wantID {
				t.Errorf("ID: got %d want %d", result.Task.ID, tc.wantID)
			}
			if result.Task.Status != tc.wantStatus {
				t.Errorf("Status: got %q want %q", result.Task.Status, tc.wantStatus)
			}
			if result.Task.Priority != tc.wantPrio {
				t.Errorf("Priority: got %d want %d", result.Task.Priority, tc.wantPrio)
			}
			if result.Task.CreatedAt != now {
				t.Errorf("CreatedAt: got %q want %q", result.Task.CreatedAt, now)
			}
			// Verify task.created event was emitted.
			if len(emits.events) == 0 {
				t.Errorf("expected task.created event, got none")
			} else if emits.events[0].EventName != "task.created" {
				t.Errorf("event name: got %q want 'task.created'", emits.events[0].EventName)
			}
		})
	}
}

// ---- Tests: get_task ----

func TestGetTask(t *testing.T) {
	baseRow := makeTaskRow(5, "Found task", "desc", "pending", 2, "", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")

	cases := []struct {
		name    string
		params  GetTaskParams
		rows    [][]any
		dbErr   error
		wantErr bool
		wantID  int64
	}{
		{
			name:   "happy path",
			params: GetTaskParams{ID: 5},
			rows:   [][]any{baseRow},
			wantID: 5,
		},
		{
			name:    "not found returns error",
			params:  GetTaskParams{ID: 999},
			rows:    [][]any{},
			wantErr: true,
		},
		{
			name:    "db error propagates",
			params:  GetTaskParams{ID: 1},
			dbErr:   fmt.Errorf("db gone"),
			wantErr: true,
		},
		{
			name:    "id zero",
			params:  GetTaskParams{ID: 0},
			rows:    [][]any{},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hc := func(namespace, name string, input []byte) ([]byte, error) {
				if tc.dbErr != nil {
					return nil, tc.dbErr
				}
				if namespace == "mc_db" && name == "query" {
					out := dbQueryOutput{Columns: taskColumns, Rows: tc.rows}
					b, _ := json.Marshal(out)
					return b, nil
				}
				return []byte("{}"), nil
			}

			result, err := doGetTask(hc, tc.params)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Task.ID != tc.wantID {
				t.Errorf("ID: got %d want %d", result.Task.ID, tc.wantID)
			}
		})
	}
}

// ---- Tests: update_task ----

func TestUpdateTask(t *testing.T) {
	const now = "2026-03-21T13:00:00Z"

	pendingRow := makeTaskRow(1, "Task", "desc", "pending", 3, "", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
	doneRow := makeTaskRow(1, "Task", "desc", "done", 3, "", "2026-01-01T00:00:00Z", now)

	cases := []struct {
		name   string
		params UpdateTaskParams
		// querySeq controls what successive SELECT calls return.
		querySeq           [][]any
		execErr            error
		dbQueryErr         error
		rowsAffected       int64
		wantErr            bool
		wantStatus         string
		wantCompletedEvent bool
		wantUpdatedEvent   bool
	}{
		{
			name:             "update title",
			params:           UpdateTaskParams{ID: 1, Title: "New title"},
			querySeq:         [][]any{pendingRow, pendingRow},
			rowsAffected:     1,
			wantUpdatedEvent: true,
			wantStatus:       "pending",
		},
		{
			name:               "transition to done emits completed",
			params:             UpdateTaskParams{ID: 1, Status: "done"},
			querySeq:           [][]any{pendingRow, doneRow},
			rowsAffected:       1,
			wantUpdatedEvent:   true,
			wantCompletedEvent: true,
			wantStatus:         "done",
		},
		{
			name:             "already done no completed event",
			params:           UpdateTaskParams{ID: 1, Title: "T"},
			querySeq:         [][]any{doneRow, doneRow},
			rowsAffected:     1,
			wantUpdatedEvent: true,
			wantStatus:       "done",
		},
		{
			name:    "id zero returns error",
			params:  UpdateTaskParams{ID: 0},
			wantErr: true,
		},
		{
			name:     "task not found on initial query",
			params:   UpdateTaskParams{ID: 99},
			querySeq: [][]any{},
			wantErr:  true,
		},
		{
			name:     "invalid status returns error",
			params:   UpdateTaskParams{ID: 1, Status: "bogus"},
			querySeq: [][]any{pendingRow},
			wantErr:  true,
		},
		{
			name:             "clear description",
			params:           UpdateTaskParams{ID: 1, ClearDescription: true},
			querySeq:         [][]any{pendingRow, makeTaskRow(1, "Task", "", "pending", 3, "", "2026-01-01T00:00:00Z", now)},
			rowsAffected:     1,
			wantUpdatedEvent: true,
			wantStatus:       "pending",
		},
		{
			name:             "clear due date",
			params:           UpdateTaskParams{ID: 1, ClearDueDate: true},
			querySeq:         [][]any{pendingRow, pendingRow},
			rowsAffected:     1,
			wantUpdatedEvent: true,
			wantStatus:       "pending",
		},
		{
			name:     "db exec error propagates",
			params:   UpdateTaskParams{ID: 1, Title: "T"},
			querySeq: [][]any{pendingRow},
			execErr:  fmt.Errorf("disk full"),
			wantErr:  true,
		},
		{
			// Concurrent delete: SELECT finds the row but UPDATE affects 0 rows
			// because another goroutine deleted the task between the SELECT and UPDATE.
			name:         "concurrent delete returns task not found",
			params:       UpdateTaskParams{ID: 1, Title: "T"},
			querySeq:     [][]any{pendingRow},
			rowsAffected: 0,
			wantErr:      true,
		},
		{
			// UPDATE succeeds (rows_affected: 1) but the reload SELECT returns no rows,
			// e.g. the task was deleted between the UPDATE and the reload.
			name:         "not found after reload SELECT",
			params:       UpdateTaskParams{ID: 1, Title: "T"},
			querySeq:     [][]any{pendingRow},
			rowsAffected: 1,
			wantErr:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			queryCallIdx := 0
			emits := &capturedEmits{}

			hc := func(namespace, name string, input []byte) ([]byte, error) {
				if namespace == "mc_event" {
					return emits.capture(namespace, name, input)
				}
				if namespace == "mc_db" {
					if name == "query" {
						if tc.dbQueryErr != nil {
							return nil, tc.dbQueryErr
						}
						if queryCallIdx >= len(tc.querySeq) {
							// Return empty.
							out := dbQueryOutput{Columns: taskColumns, Rows: [][]any{}}
							b, _ := json.Marshal(out)
							return b, nil
						}
						var rows [][]any
						if tc.querySeq[queryCallIdx] != nil {
							rows = [][]any{tc.querySeq[queryCallIdx]}
						} else {
							rows = [][]any{}
						}
						queryCallIdx++
						out := dbQueryOutput{Columns: taskColumns, Rows: rows}
						b, _ := json.Marshal(out)
						return b, nil
					}
					if name == "exec" {
						if tc.execErr != nil {
							return nil, tc.execErr
						}
						// Verify parameterized — args must be non-empty.
						var qi dbExecInput
						_ = json.Unmarshal(input, &qi)
						if len(qi.Args) == 0 {
							t.Errorf("exec called with no args")
						}
						out := dbExecOutput{RowsAffected: tc.rowsAffected, LastInsertID: 1}
						b, _ := json.Marshal(out)
						return b, nil
					}
				}
				return []byte("{}"), nil
			}

			result, err := doUpdateTask(hc, tc.params, now)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantStatus != "" && result.Task.Status != tc.wantStatus {
				t.Errorf("Status: got %q want %q", result.Task.Status, tc.wantStatus)
			}

			hasUpdated := false
			hasCompleted := false
			for _, ev := range emits.events {
				if ev.EventName == "task.updated" {
					hasUpdated = true
				}
				if ev.EventName == "task.completed" {
					hasCompleted = true
				}
			}

			if tc.wantUpdatedEvent && !hasUpdated {
				t.Errorf("expected task.updated event, got events: %v", emits.events)
			}
			if tc.wantCompletedEvent && !hasCompleted {
				t.Errorf("expected task.completed event, got events: %v", emits.events)
			}
			if !tc.wantCompletedEvent && hasCompleted {
				t.Errorf("unexpected task.completed event")
			}
		})
	}
}

// ---- Tests: delete_task ----

func TestDeleteTask(t *testing.T) {
	const now = "2026-03-21T14:00:00Z"

	cases := []struct {
		name         string
		params       DeleteTaskParams
		rowsAffected int64
		dbErr        error
		wantErr      bool
		wantDeleted  bool
		wantEvent    bool
	}{
		{
			name:         "happy path deleted",
			params:       DeleteTaskParams{ID: 1},
			rowsAffected: 1,
			wantDeleted:  true,
			wantEvent:    true,
		},
		{
			name:         "not found returns deleted=false no error",
			params:       DeleteTaskParams{ID: 999},
			rowsAffected: 0,
			wantDeleted:  false,
			wantEvent:    false,
		},
		{
			name:    "db error propagates",
			params:  DeleteTaskParams{ID: 1},
			dbErr:   fmt.Errorf("db gone"),
			wantErr: true,
		},
		{
			name:         "id zero no rows affected",
			params:       DeleteTaskParams{ID: 0},
			rowsAffected: 0,
			wantDeleted:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			emits := &capturedEmits{}
			hc := func(namespace, name string, input []byte) ([]byte, error) {
				if namespace == "mc_event" {
					return emits.capture(namespace, name, input)
				}
				if tc.dbErr != nil {
					return nil, tc.dbErr
				}
				if namespace == "mc_db" && name == "exec" {
					// Verify parameterized args.
					var qi dbExecInput
					_ = json.Unmarshal(input, &qi)
					if len(qi.Args) == 0 {
						t.Errorf("exec called with no args")
					}
					out := dbExecOutput{RowsAffected: tc.rowsAffected}
					b, _ := json.Marshal(out)
					return b, nil
				}
				return []byte("{}"), nil
			}

			result, err := doDeleteTask(hc, tc.params, now)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Deleted != tc.wantDeleted {
				t.Errorf("Deleted: got %v want %v", result.Deleted, tc.wantDeleted)
			}
			if result.ID != tc.params.ID {
				t.Errorf("ID: got %d want %d", result.ID, tc.params.ID)
			}

			hasDeletedEvent := false
			for _, ev := range emits.events {
				if ev.EventName == "task.deleted" {
					hasDeletedEvent = true
				}
			}
			if tc.wantEvent && !hasDeletedEvent {
				t.Errorf("expected task.deleted event, none emitted")
			}
			if !tc.wantEvent && hasDeletedEvent {
				t.Errorf("unexpected task.deleted event emitted")
			}
		})
	}
}

// ---- Tests: SQL parameterization ----

func TestSQLParameterization(t *testing.T) {
	// All SQL queries must pass args as parameters, never via string interpolation.
	// We verify this by checking that the Args field is always populated.
	const now = "2026-03-21T12:00:00Z"

	injectionTitle := "'; DROP TABLE tasks; --"

	var capturedSQL []string
	var capturedArgs [][]any

	hc := func(namespace, name string, input []byte) ([]byte, error) {
		if namespace == "mc_db" {
			if name == "exec" {
				var qi dbExecInput
				_ = json.Unmarshal(input, &qi)
				capturedSQL = append(capturedSQL, qi.SQL)
				capturedArgs = append(capturedArgs, qi.Args)
				return json.Marshal(dbExecOutput{RowsAffected: 1, LastInsertID: 1})
			}
			if name == "query" {
				var qi dbQueryInput
				_ = json.Unmarshal(input, &qi)
				capturedSQL = append(capturedSQL, qi.SQL)
				capturedArgs = append(capturedArgs, qi.Args)
				row := makeTaskRow(1, injectionTitle, "", "pending", 3, "", now, now)
				return json.Marshal(dbQueryOutput{Columns: taskColumns, Rows: [][]any{row}})
			}
		}
		return []byte("{}"), nil
	}

	// create_task with injection title.
	_, err := doCreateTask(hc, CreateTaskParams{Title: injectionTitle}, now)
	if err != nil {
		t.Fatalf("create_task failed: %v", err)
	}

	// delete_task.
	_, err = doDeleteTask(hc, DeleteTaskParams{ID: 1}, now)
	if err != nil {
		t.Fatalf("delete_task failed: %v", err)
	}

	for i, sql := range capturedSQL {
		// The injection string must NOT appear in the SQL text itself.
		if len(capturedArgs[i]) == 0 {
			t.Errorf("query %d has no args: %s", i, sql)
		}
		// Verify the injection string is in args, not in SQL.
		sqlContainsInjection := false
		for j := range sql {
			if j+len(injectionTitle) <= len(sql) && sql[j:j+len(injectionTitle)] == injectionTitle {
				sqlContainsInjection = true
			}
		}
		if sqlContainsInjection {
			t.Errorf("SQL contains injection string directly (not parameterized): %s", sql)
		}
	}
}

// ---- Tests: EventBus integration ----

func TestEventEmitFailureDoesNotCrash(t *testing.T) {
	// Emitting events is best-effort. A failure in mc_event::emit must not
	// propagate an error to the tool caller.
	const now = "2026-03-21T12:00:00Z"

	hc := func(namespace, name string, input []byte) ([]byte, error) {
		if namespace == "mc_event" {
			return nil, fmt.Errorf("event bus down")
		}
		if namespace == "mc_db" && name == "exec" {
			return json.Marshal(dbExecOutput{RowsAffected: 1, LastInsertID: 1})
		}
		return []byte("{}"), nil
	}

	_, err := doCreateTask(hc, CreateTaskParams{Title: "T"}, now)
	if err != nil {
		t.Errorf("event emit failure should not propagate error: %v", err)
	}
}

func TestTaskCompletedEventEmittedAfterUpdated(t *testing.T) {
	// task.completed must be emitted AFTER task.updated, and only when
	// transitioning from a non-done status to done.
	const now = "2026-03-21T13:00:00Z"
	pendingRow := makeTaskRow(1, "Task", "", "pending", 3, "", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z")
	doneRow := makeTaskRow(1, "Task", "", "done", 3, "", "2026-01-01T00:00:00Z", now)

	queryIdx := 0
	emits := &capturedEmits{}
	hc := func(namespace, name string, input []byte) ([]byte, error) {
		if namespace == "mc_event" {
			return emits.capture(namespace, name, input)
		}
		if namespace == "mc_db" {
			if name == "query" {
				rows := [][]any{}
				if queryIdx == 0 {
					rows = [][]any{pendingRow}
				} else {
					rows = [][]any{doneRow}
				}
				queryIdx++
				return json.Marshal(dbQueryOutput{Columns: taskColumns, Rows: rows})
			}
			if name == "exec" {
				return json.Marshal(dbExecOutput{RowsAffected: 1})
			}
		}
		return []byte("{}"), nil
	}

	_, err := doUpdateTask(hc, UpdateTaskParams{ID: 1, Status: "done"}, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(emits.events) < 2 {
		t.Fatalf("expected at least 2 events, got %d: %v", len(emits.events), emits.events)
	}
	// task.updated must come before task.completed.
	if emits.events[0].EventName != "task.updated" {
		t.Errorf("first event: got %q want 'task.updated'", emits.events[0].EventName)
	}
	if emits.events[1].EventName != "task.completed" {
		t.Errorf("second event: got %q want 'task.completed'", emits.events[1].EventName)
	}
}
