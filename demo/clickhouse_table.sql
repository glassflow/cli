CREATE TABLE IF NOT EXISTS demo_events
(
    event_id UUID,
    type String,
    source String,
    created_at DateTime
)
ENGINE = MergeTree
ORDER BY (event_id)

