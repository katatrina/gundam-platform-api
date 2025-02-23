-- Function to update timestamp
CREATE
OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at
= NOW();
RETURN NEW;
END;
$$
LANGUAGE plpgsql;

CREATE
OR REPLACE FUNCTION create_timestamp_triggers()
RETURNS void AS $$
DECLARE
table_name text;
BEGIN
FOR table_name IN
SELECT unnest(ARRAY[
                  'users',
              'user_addresses',
              'gundams',
              'orders'
                  ])
           LOOP EXECUTE format('
            CREATE TRIGGER update_timestamp_trigger
            BEFORE UPDATE ON %I
            FOR EACH ROW EXECUTE FUNCTION update_timestamp();
        ', table_name);
END LOOP;
END;
$$
LANGUAGE plpgsql;

-- Chạy function để tạo triggers
SELECT create_timestamp_triggers();