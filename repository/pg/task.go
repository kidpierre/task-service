package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgerrcode"

	"task-service/entity"
)

const (
	taskColumns = `id, title, description, status, deadline, creator_id, assignee_id, created_at, updated_at`

	taskFilterCondition = `
		WHERE ($1::text IS NULL OR status = $1)
		  AND ($2::bigint IS NULL OR assignee_id = $2)
		  AND ($3::timestamptz IS NULL OR deadline >= $3)
		  AND ($4::timestamptz IS NULL OR deadline <= $4)`

	assigneeForeignKey = "tasks_assignee_id_fkey"
)

type TaskRepo struct {
	db *sql.DB
}

func NewTaskRepo(db *sql.DB) *TaskRepo {
	return &TaskRepo{db: db}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTask(row rowScanner) (entity.Task, error) {
	var t entity.Task
	err := row.Scan(&t.ID, &t.Title, &t.Description, &t.Status, &t.Deadline,
		&t.CreatorID, &t.AssigneeID, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (r *TaskRepo) Create(ctx context.Context, task entity.Task) (entity.Task, error) {
	const query = `
		INSERT INTO tasks (title, description, status, deadline, creator_id, assignee_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + taskColumns

	created, err := scanTask(r.db.QueryRowContext(ctx, query,
		task.Title, task.Description, task.Status, task.Deadline, task.CreatorID, task.AssigneeID))
	if err != nil {
		return entity.Task{}, mapWriteError(err, "insert task")
	}
	return created, nil
}

func (r *TaskRepo) GetByID(ctx context.Context, id int64) (entity.Task, error) {
	const query = `SELECT ` + taskColumns + ` FROM tasks WHERE id = $1`

	task, err := scanTask(r.db.QueryRowContext(ctx, query, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return entity.Task{}, entity.ErrTaskNotFound
		}
		return entity.Task{}, fmt.Errorf("select task: %w", err)
	}
	return task, nil
}

func (r *TaskRepo) List(ctx context.Context, filter entity.TaskFilter) ([]entity.Task, int, error) {
	const (
		listQuery = `SELECT ` + taskColumns + ` FROM tasks` + taskFilterCondition + `
		ORDER BY deadline, id
		LIMIT $5 OFFSET $6`
		countQuery = `SELECT count(*) FROM tasks` + taskFilterCondition
	)

	args := []any{filter.Status, filter.AssigneeID, filter.DeadlineFrom, filter.DeadlineTo}

	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count tasks: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, listQuery, append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("select tasks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tasks := make([]entity.Task, 0, filter.Limit)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan task: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate tasks: %w", err)
	}

	return tasks, total, nil
}

func (r *TaskRepo) Update(ctx context.Context, id int64, upd entity.TaskUpdate) (entity.Task, error) {
	const query = `
		UPDATE tasks
		SET title       = COALESCE($2, title),
		    description = COALESCE($3, description),
		    status      = COALESCE($4, status),
		    deadline    = COALESCE($5, deadline),
		    assignee_id = COALESCE($6, assignee_id),
		    updated_at  = now()
		WHERE id = $1
		RETURNING ` + taskColumns

	updated, err := scanTask(r.db.QueryRowContext(ctx, query,
		id, upd.Title, upd.Description, upd.Status, upd.Deadline, upd.AssigneeID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return entity.Task{}, entity.ErrTaskNotFound
		}
		return entity.Task{}, mapWriteError(err, "update task")
	}
	return updated, nil
}

func (r *TaskRepo) Delete(ctx context.Context, id int64) error {
	const query = `DELETE FROM tasks WHERE id = $1`

	res, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if n == 0 {
		return entity.ErrTaskNotFound
	}
	return nil
}

func (r *TaskRepo) MarkOverdue(ctx context.Context, now time.Time) (int64, error) {
	const query = `
		UPDATE tasks
		SET status = $1, updated_at = now()
		WHERE deadline < $2 AND status IN ($3, $4)`

	res, err := r.db.ExecContext(ctx, query,
		entity.StatusOverdue, now, entity.StatusCreated, entity.StatusInProgress)
	if err != nil {
		return 0, fmt.Errorf("mark overdue tasks: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("mark overdue tasks: %w", err)
	}
	return n, nil
}

func mapWriteError(err error, op string) error {
	if name, ok := constraintViolation(err, pgerrcode.ForeignKeyViolation); ok && name == assigneeForeignKey {
		return entity.ErrAssigneeNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}
