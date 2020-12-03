USE costbasis;

DROP TABLE IF EXISTS test_table;

CREATE TABLE test_table (
  label        VARCHAR(32) NOT NULL PRIMARY KEY,
  time_seconds DATETIME NOT NULL,
  time_millis  DATETIME(3) NOT NULL
);

INSERT INTO test_table
  (label, time_seconds, time_millis)
VALUES
  ('INSERT string times', '2020-04-02 05:16:08.987', '2020-04-02 05:16:08.987'),
  ('INSERT unix times', FROM_UNIXTIME(1585804568.987), FROM_UNIXTIME(1585804568.987));

LOAD DATA
LOCAL INFILE 'lines.tsv'
INTO TABLE test_table
  (label, time_seconds, time_millis);

SELECT * FROM test_table;
