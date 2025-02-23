-- Drop triggers trước
DROP TRIGGER IF EXISTS enforce_max_addresses ON user_addresses;
DROP TRIGGER IF EXISTS prevent_delete_important_address ON user_addresses;

-- Drop functions
DROP FUNCTION IF EXISTS check_max_addresses;
DROP FUNCTION IF EXISTS prevent_delete_important_address;

-- Drop indexes
DROP INDEX IF EXISTS unique_primary_address;
DROP INDEX IF EXISTS unique_pickup_address;