-- +goose Up
CREATE TABLE IF NOT EXISTS donor_adjustments (
    person_id VARCHAR,
    display_name VARCHAR,
    slug VARCHAR,
    amount REAL,
    PRIMARY KEY (person_id, slug)
);
CREATE INDEX person_id_idx ON donor_adjustments (person_id);

-- +goose Down
DROP TABLE IF EXISTS donor_adjustments;