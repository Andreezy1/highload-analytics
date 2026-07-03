CREATE TABLE IF NOT EXISTS events (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    event_type VARCHAR(255) NOT NULL,
    time TIMESTAMPTZ NOT NULL,
    page_url VARCHAR(255) NOT NULL
);

-- Индекс для оптимизации аналитических выборок по времени и пользователям
CREATE INDEX IF NOT EXISTS idx_events_time ON events(time);
CREATE INDEX IF NOT EXISTS idx_events_user_id ON events(user_id);