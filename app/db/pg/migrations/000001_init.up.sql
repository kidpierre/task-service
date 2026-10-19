CREATE TABLE users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tasks (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title       TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    status      TEXT        NOT NULL CHECK (status IN ('created', 'in_progress', 'done', 'overdue')),
    deadline    TIMESTAMPTZ NOT NULL,
    creator_id  BIGINT      NOT NULL REFERENCES users (id),
    assignee_id BIGINT      NOT NULL REFERENCES users (id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX tasks_creator_id_idx ON tasks (creator_id);
CREATE INDEX tasks_assignee_id_idx ON tasks (assignee_id);
CREATE INDEX tasks_deadline_idx ON tasks (deadline);
CREATE INDEX tasks_status_deadline_idx ON tasks (status, deadline);
