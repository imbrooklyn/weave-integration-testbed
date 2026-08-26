-- Stable SQL fixture schema for local demonstrations and compatibility checks.
DROP TABLE IF EXISTS semantic_records;

-- weave:split
CREATE TABLE semantic_records (
    id VARCHAR(16) COLLATE "C" NOT NULL PRIMARY KEY,
    number_value BIGINT NOT NULL,
    decimal_value NUMERIC(10, 3) NOT NULL,
    text_value TEXT COLLATE "C" NOT NULL,
    bool_value BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ(6) NOT NULL,
    nullable_number BIGINT NULL,
    nullable_text TEXT COLLATE "C" NULL,
    equality_only_text TEXT COLLATE "C" NOT NULL
);

-- weave:split
CREATE INDEX semantic_records_created_at_idx ON semantic_records (created_at);
