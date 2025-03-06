-- Drop the triggers first
DROP TRIGGER IF EXISTS update_timestamp_trigger ON users;
DROP TRIGGER IF EXISTS update_timestamp_trigger ON user_addresses;
DROP TRIGGER IF EXISTS update_timestamp_trigger ON gundams;
DROP TRIGGER IF EXISTS update_timestamp_trigger ON user_subscriptions;
DROP TRIGGER IF EXISTS update_timestamp_trigger ON orders;

-- Drop the helper function that created the triggers
DROP FUNCTION IF EXISTS create_timestamp_triggers();

-- Drop the update_timestamp function
DROP FUNCTION IF EXISTS update_timestamp();