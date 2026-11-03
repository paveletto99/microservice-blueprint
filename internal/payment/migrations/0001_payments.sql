CREATE TABLE IF NOT EXISTS simulated_payments (
  id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  idempotency_digest CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  order_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  amount_minor BIGINT NOT NULL,
  currency CHAR(3) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  PRIMARY KEY (idempotency_digest),
  UNIQUE KEY payment_id (id)
) ENGINE=InnoDB
