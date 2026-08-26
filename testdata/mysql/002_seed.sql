-- Replaying this file restores the complete stable fixture.
DELETE FROM semantic_records;

-- weave:split
INSERT INTO semantic_records (
    id,
    number_value,
    decimal_value,
    text_value,
    bool_value,
    created_at,
    nullable_number,
    nullable_text,
    equality_only_text
) VALUES
    (
        'r01', 1, 1.125, 'plain-start', TRUE,
        '2024-01-01 00:00:00.000000', 1, 'plain-start', 'plain-start'
    ),
    (
        'r02', 2, 2.250,
        CONCAT('literal %_! .*+?[](){}^$|', CHAR(92), ' 世界', CHAR(10), 'end'),
        FALSE, '2024-01-02 00:00:00.000000', 2,
        CONCAT('literal %_! .*+?[](){}^$|', CHAR(92), ' 世界', CHAR(10), 'end'),
        CONCAT('literal %_! .*+?[](){}^$|', CHAR(92), ' 世界', CHAR(10), 'end')
    ),
    (
        'r03', 3, 3.375, 'prefix-middle-suffix', TRUE,
        '2024-01-03 00:00:00.000000', NULL, NULL, 'prefix-middle-suffix'
    ),
    (
        'r04', 4, 4.500, 'prefix %_ suffix', FALSE,
        '2024-01-04 00:00:00.000000', NULL, NULL, 'prefix %_ suffix'
    ),
    (
        'r05', 5, 5.625, '世界-end', TRUE,
        '2024-01-05 00:00:00.000000', 5, '世界-end', '世界-end'
    ),
    (
        'r06', 6, 6.750, '.*', FALSE,
        '2024-01-06 00:00:00.000000', 2, '.*', '.*'
    );
