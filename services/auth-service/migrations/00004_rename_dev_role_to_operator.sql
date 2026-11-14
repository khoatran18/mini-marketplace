-- +goose Up
-- The privileged "dev" role is now called "operator" (it never meant a developer: it reads the system and proposes ops actions).
-- Existing accounts keep their username and password; their sessions (JWT with role "dev") must sign in again.
UPDATE accounts SET role = 'operator' WHERE role = 'dev';

-- +goose Down
UPDATE accounts SET role = 'dev' WHERE role = 'operator';
