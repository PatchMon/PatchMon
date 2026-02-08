-- Reconcile user_sessions migration from 1.2.7 to 1.2.8+
-- This migration handles the case where 1.2.7 had 'add_user_sessions' without timestamp
-- and 1.2.8+ renamed it to '20251005000000_add_user_sessions' with timestamp

DO $$
DECLARE
    table_exists boolean := false;
    migration_exists boolean := false;
BEGIN
    -- Check if user_sessions table exists
    SELECT EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_schema = 'public' 
        AND table_name = 'user_sessions'
    ) INTO table_exists;
    
    -- Check if the migration record already exists
    SELECT EXISTS (
        SELECT 1 FROM _prisma_migrations 
        WHERE migration_name = '20251005000000_add_user_sessions'
    ) INTO migration_exists;
    
    -- If table exists but no migration record, create one
    IF table_exists AND NOT migration_exists THEN
        RAISE NOTICE 'Table exists but no migration record found - creating migration record for 1.2.7 upgrade';
        
        -- Insert a successful migration record for the existing table
        INSERT INTO _prisma_migrations (
            id, 
            checksum, 
            finished_at, 
            migration_name, 
            logs, 
            rolled_back_at, 
            started_at, 
            applied_steps_count
        ) VALUES (
            gen_random_uuid()::text,
            '', -- Empty checksum since we're reconciling
            NOW(),
            '20251005000000_add_user_sessions',
            'Reconciled from 1.2.7 - table already exists',
            NULL,
            NOW(),
            1
        );
        
        RAISE NOTICE 'Migration record created for existing table';
    ELSIF table_exists AND migration_exists THEN
        RAISE NOTICE 'Table exists and migration record exists - no action needed';
    ELSE
        RAISE NOTICE 'Table does not exist - migration will proceed normally';
    END IF;
    
    -- Additional check: If we have any old migration names, update them
    IF EXISTS (SELECT 1 FROM _prisma_migrations WHERE migration_name = 'add_user_sessions') THEN
        RAISE NOTICE 'Found old migration name - updating to new format';
        UPDATE _prisma_migrations 
        SET migration_name = '20251005000000_add_user_sessions'
        WHERE migration_name = 'add_user_sessions';
        RAISE NOTICE 'Old migration name updated';
    END IF;
    
END $$;
