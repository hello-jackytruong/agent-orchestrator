-- +goose Up
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_model_usage_events_created_at_binding
    ON model_usage_events (created_at, binding_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_model_usage_events_created_at_binding;
-- +goose StatementEnd
