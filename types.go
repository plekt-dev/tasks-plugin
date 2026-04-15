// types.go defines all domain types, MCP tool parameter/result types,
// and host-call payload types for the tasks-plugin.
// This file has no build constraints so it is shared by both the wasip1
// build (plugin.go) and the host-side test build (plugin_test.go).

package main

// ---- Domain types ----

// Task is the core domain type for a task record.
type Task struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	DueDate     string `json:"due_date,omitempty"`
	ProjectID   *int64 `json:"project_id,omitempty"`
	TypeID      *int64 `json:"type_id,omitempty"`
	AssigneeID  *int64 `json:"assignee_id,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// BoardColumnDef represents a user-defined board status column.
type BoardColumnDef struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Label     string `json:"label"`
	Position  int    `json:"position"`
	Color     string `json:"color,omitempty"`
	ProjectID *int64 `json:"project_id,omitempty"`
	IsDefault bool   `json:"is_default"`
	CreatedAt string `json:"created_at"`
}

// TaskType represents a user-defined task type.
type TaskType struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Label     string `json:"label"`
	Color     string `json:"color,omitempty"`
	Icon      string `json:"icon,omitempty"`
	ProjectID *int64 `json:"project_id,omitempty"`
	CreatedAt string `json:"created_at"`
}

// Comment represents a comment on a task.
type Comment struct {
	ID         int64  `json:"id"`
	TaskID     int64  `json:"task_id"`
	AuthorType string `json:"author_type"`
	AuthorID   *int64 `json:"author_id,omitempty"`
	AuthorName string `json:"author_name"`
	Body       string `json:"body"`
	CreatedAt  string `json:"created_at"`
}

// ---- MCP tool input/output types ----

// ListTasksParams holds optional filters for the list_tasks tool.
type ListTasksParams struct {
	StatusFilter string `json:"status_filter,omitempty"`
	PriorityMin  int    `json:"priority_min,omitempty"`
	PriorityMax  int    `json:"priority_max,omitempty"`
	ProjectID    *int64 `json:"project_id,omitempty"`
	TypeID       *int64 `json:"type_id,omitempty"`
	AssigneeID   *int64 `json:"assignee_id,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

// ListTasksResult is the output of the list_tasks tool.
type ListTasksResult struct {
	Tasks []Task `json:"tasks"`
	Total int    `json:"total"`
}

// CreateTaskParams holds parameters for the create_task tool.
type CreateTaskParams struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
	Priority    int    `json:"priority,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	ProjectID   *int64 `json:"project_id,omitempty"`
	TypeID      *int64 `json:"type_id,omitempty"`
	AssigneeID  *int64 `json:"assignee_id,omitempty"`
}

// CreateTaskResult is the output of the create_task tool.
type CreateTaskResult struct {
	Task   Task   `json:"task"`
	WebURL string `json:"web_url,omitempty"`
}

// GetTaskParams holds parameters for the get_task tool.
type GetTaskParams struct {
	ID int64 `json:"id"`
}

// GetTaskResult is the output of the get_task tool.
type GetTaskResult struct {
	Task Task `json:"task"`
}

// UpdateTaskParams holds parameters for the update_task tool.
type UpdateTaskParams struct {
	ID               int64  `json:"id"`
	Title            string `json:"title,omitempty"`
	Description      string `json:"description,omitempty"`
	ClearDescription bool   `json:"clear_description,omitempty"`
	Status           string `json:"status,omitempty"`
	Priority         int    `json:"priority,omitempty"`
	DueDate          string `json:"due_date,omitempty"`
	ClearDueDate     bool   `json:"clear_due_date,omitempty"`
	TypeID           *int64 `json:"type_id,omitempty"`
	ClearType        bool   `json:"clear_type,omitempty"`
	AssigneeID       *int64 `json:"assignee_id,omitempty"`
	ClearAssignee    bool   `json:"clear_assignee,omitempty"`
}

// UpdateTaskResult is the output of the update_task tool.
type UpdateTaskResult struct {
	Task Task `json:"task"`
}

// DeleteTaskParams holds parameters for the delete_task tool.
type DeleteTaskParams struct {
	ID int64 `json:"id"`
}

// DeleteTaskResult is the output of the delete_task tool.
type DeleteTaskResult struct {
	Deleted bool  `json:"deleted"`
	ID      int64 `json:"id"`
}

// ---- Board column CRUD types ----

type ListBoardColumnsResult struct {
	Columns []BoardColumnDef `json:"columns"`
}

type CreateBoardColumnParams struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Position  int    `json:"position,omitempty"`
	Color     string `json:"color,omitempty"`
	ProjectID *int64 `json:"project_id,omitempty"`
}

type CreateBoardColumnResult struct {
	Column BoardColumnDef `json:"column"`
}

type UpdateBoardColumnParams struct {
	ID       int64  `json:"id"`
	Label    string `json:"label,omitempty"`
	Position *int   `json:"position,omitempty"`
	Color    string `json:"color,omitempty"`
}

type UpdateBoardColumnResult struct {
	Column BoardColumnDef `json:"column"`
}

type DeleteBoardColumnParams struct {
	ID int64 `json:"id"`
}

type DeleteBoardColumnResult struct {
	Deleted bool  `json:"deleted"`
	ID      int64 `json:"id"`
}

// ---- Task type CRUD types ----

type ListTaskTypesResult struct {
	Types []TaskType `json:"types"`
}

type CreateTaskTypeParams struct {
	Name      string `json:"name"`
	Label     string `json:"label"`
	Color     string `json:"color,omitempty"`
	Icon      string `json:"icon,omitempty"`
	ProjectID *int64 `json:"project_id,omitempty"`
}

type CreateTaskTypeResult struct {
	Type TaskType `json:"type"`
}

type UpdateTaskTypeParams struct {
	ID    int64  `json:"id"`
	Label string `json:"label,omitempty"`
	Color string `json:"color,omitempty"`
	Icon  string `json:"icon,omitempty"`
}

type UpdateTaskTypeResult struct {
	Type TaskType `json:"type"`
}

type DeleteTaskTypeParams struct {
	ID int64 `json:"id"`
}

type DeleteTaskTypeResult struct {
	Deleted bool  `json:"deleted"`
	ID      int64 `json:"id"`
}

// ---- Comment CRUD types ----

type ListCommentsParams struct {
	TaskID int64 `json:"task_id"`
}

type ListCommentsResult struct {
	Comments []Comment `json:"comments"`
	Total    int       `json:"total"`
}

type CreateCommentParams struct {
	TaskID     int64  `json:"task_id"`
	AuthorName string `json:"author_name"`
	AuthorType string `json:"author_type,omitempty"`
	AuthorID   *int64 `json:"author_id,omitempty"`
	Body       string `json:"body"`
	McAuthor   string `json:"_mc_author,omitempty"`
}

type CreateCommentResult struct {
	Comment Comment `json:"comment"`
}

type DeleteCommentParams struct {
	ID int64 `json:"id"`
}

type DeleteCommentResult struct {
	Deleted bool  `json:"deleted"`
	ID      int64 `json:"id"`
}

type UpdateCommentParams struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
}

// ---- Task detail (enriched) ----

type TaskDetailResult struct {
	Task      Task             `json:"task"`
	Comments  []Comment        `json:"comments"`
	TypeInfo  *TaskType        `json:"type_info,omitempty"`
	Columns   []BoardColumnDef `json:"columns"`
	TaskTypes []TaskType       `json:"task_types"`
}

// ---- Dashboard widget types ----

// TaskOverviewItem is a lightweight task record for the dashboard widget.
// ProjectID is intentionally serialized so the dashboard widget can resolve
// its link_template ("/p/projects-plugin/project/{project_id}/project-tasks?task={id}").
// Tasks without a project (project_id IS NULL) render as plain text — there is
// no per-task URL for orphan tasks.
type TaskOverviewItem struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Priority  int    `json:"priority"`
	ProjectID *int64 `json:"project_id,omitempty"`
	CreatedAt string `json:"created_at"`
}

// TaskOverviewResult is the output of the get_task_overview function.
type TaskOverviewResult struct {
	Tasks []TaskOverviewItem `json:"tasks"`
	Total int                `json:"total"`
}

// ---- Kanban board types ----

// BoardColumn represents one column in the kanban board view.
type BoardColumn struct {
	ID        string `json:"id"`
	DbID      int64  `json:"db_id"`
	Title     string `json:"title"`
	Color     string `json:"color,omitempty"`
	IsDefault bool   `json:"is_default,omitempty"`
	Position  int    `json:"position"`
	Items     []Task `json:"items"`
}

// BoardResult is the output of the get_tasks_board function.
type BoardResult struct {
	Columns   []BoardColumn `json:"columns"`
	TaskTypes []TaskType    `json:"task_types"`
}

// ---- Host function call payload types ----

// dbQueryInput is the JSON payload sent to mc_db::query.
type dbQueryInput struct {
	SQL  string `json:"sql"`
	Args []any  `json:"args"`
}

// dbQueryOutput is the JSON payload received from mc_db::query.
type dbQueryOutput struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// dbExecInput is the JSON payload sent to mc_db::exec.
type dbExecInput struct {
	SQL  string `json:"sql"`
	Args []any  `json:"args"`
}

// dbExecOutput is the JSON payload received from mc_db::exec.
type dbExecOutput struct {
	RowsAffected int64 `json:"rows_affected"`
	LastInsertID int64 `json:"last_insert_id"`
}

// emitInput is the JSON payload sent to mc_event::emit.
type emitInput struct {
	EventName string `json:"event_name"`
	Payload   any    `json:"payload"`
}
