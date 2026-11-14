-- +goose Up
-- A username is bound to a single role for life (the role is chosen at registration, login takes it from the account).
-- Databases that already hold the same username under several roles keep working: the index is skipped (with a notice)
-- and the service layer still refuses new duplicates; clean the rows up and create the index by hand.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM accounts GROUP BY username HAVING COUNT(*) > 1) THEN
    RAISE NOTICE 'accounts has duplicate usernames across roles: uni_accounts_username not created';
  ELSE
    CREATE UNIQUE INDEX IF NOT EXISTS uni_accounts_username ON accounts (username);
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP INDEX IF EXISTS uni_accounts_username;
