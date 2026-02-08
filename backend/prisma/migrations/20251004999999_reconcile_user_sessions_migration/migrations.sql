-- Reconcile user_sessions migration from 1.2.7 to 1.2.8+
-- This migration handles the case where 1.2.7 had 'add_user_sessions' without timestamp
-- and 1.2.8+ renamed it to '20251005000000_add_user_sessions' with timestamp

DO $$
DECLARE
    old_migration_exists boolean := false;
    table_exists boolean := false;
    failed_migration_exists boolean := false;
BEGIN
    -- Check if the old migration name exists
    SELECT EXISTS (
        SELECT 1 FROM _prisma_migrations 
        WHERE migration_name = 'add_user_sessions'
    ) INTO old_migration_exists;
    
    -- Check if user_sessions table exists
    SELECT EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_schema = 'public' 
        AND table_name = 'user_sessions'
    ) INTO table_exists;
    
    -- Check if there's a failed migration attempt
    SELECT EXISTS (
        SELECT 1 FROM _prisma_migrations 
        WHERE migration_name = '20251005000000_add_user_sessions' 
        AND finished_at IS NULL
    ) INTO failed_migration_exists;
    
    -- Scenario 1: Old migration exists, table exists, no failed migration
    -- This means 1.2.7 was installed and we need to update the migration name
    IF old_migration_exists AND table_exists AND NOT failed_migration_exists THEN
        RAISE NOTICE 'Found 1.2.7 migration "add_user_sessions" - updating to timestamped version';
        
        -- Update the old migration name to the new timestamped version
        UPDATE _prisma_migrations 
        SET migration_name = '20251005000000_add_user_sessions'
        WHERE migration_name = 'add_user_sessions';
        
        RAISE NOTICE 'Migration name updated: add_user_sessions -> 20251005000000_add_user_sessions';
    END IF;
    
    -- Scenario 2: Failed migration exists (upgrade attempt gone wrong)
    IF failed_migration_exists THEN
        RAISE NOTICE 'Found failed migration attempt - cleaning up';
        
        -- If table exists, it means the migration partially succeeded
        IF table_exists THEN
            RAISE NOTICE 'Table exists - marking migration as applied';
            
            -- Delete the failed migration record
            DELETE FROM _prisma_migrations 
            WHERE migration_name = '20251005000000_add_user_sessions' 
            AND finished_at IS NULL;
            
            -- Insert a successful migration record
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
                NULL,
                NULL,
                NOW(),
                1
            );
            
            RAISE NOTICE 'Migration marked as successfully applied';
        ELSE
            RAISE NOTICE 'Table does not exist - removing failed migration to allow retry';
            
            -- Just delete the failed migration to allow it to retry
            DELETE FROM _prisma_migrations 
            WHERE migration_name = '20251005000000_add_user_sessions' 
            AND finished_at IS NULL;
            
            RAISE NOTICE 'Failed migration removed - will retry on next migration run';
        END IF;
    END IF;
    
    -- Scenario 3: Everything is clean (fresh install or already reconciled)
    IF NOT old_migration_exists AND NOT failed_migration_exists THEN
        RAISE NOTICE 'No migration reconciliation needed';
    END IF;
    
END $$;
