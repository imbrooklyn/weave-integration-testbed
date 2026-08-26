-- Stable SQL fixture schema for local demonstrations and compatibility checks.
DROP TABLE IF EXISTS semantic_records;

-- weave:split
CREATE TABLE semantic_records (
    id VARCHAR(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    number_value BIGINT NOT NULL,
    decimal_value DECIMAL(10, 3) NOT NULL,
    text_value TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    bool_value BOOLEAN NOT NULL,
    created_at TIMESTAMP(6) NOT NULL,
    nullable_number BIGINT NULL,
    nullable_text TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL,
    equality_only_text TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    PRIMARY KEY (id),
    INDEX semantic_records_created_at_idx (created_at)
) ENGINE=InnoDB DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;
