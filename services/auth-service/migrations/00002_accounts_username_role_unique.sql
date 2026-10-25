-- +goose Up
-- R5: đăng nhập/đăng ký/refresh đều tra theo (username, role) nên ràng buộc duy nhất phải là cặp này,
-- không phải riêng username. Cùng username có thể tồn tại ở role khác (vd. buyer "an" và seller "an").
ALTER TABLE accounts DROP CONSTRAINT IF EXISTS uni_accounts_username;
CREATE UNIQUE INDEX IF NOT EXISTS uni_accounts_username_role ON accounts (username, role);

-- +goose Down
DROP INDEX IF EXISTS uni_accounts_username_role;
ALTER TABLE accounts ADD CONSTRAINT uni_accounts_username UNIQUE (username);
