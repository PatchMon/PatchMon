-- Make password_hash nullable for OIDC users
-- This migration is safe to run multiple times and preserves existing data

-- Check if password_hash is NOT NULL and make it nullable
DO $$ 
BEGIN
    -- Check if the column exists and is NOT NULL
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'users' 
        AND column_name = 'password_hash'
        AND is_nullable = 'NO'
    ) THEN
        ALTER TABLE "users" ALTER COLUMN "password_hash" DROP NOT NULL;
        RAISE NOTICE 'Made password_hash nullable for OIDC users';
    ELSE
        RAISE NOTICE 'password_hash is already nullable or does not exist';
    END IF;
END $$;

