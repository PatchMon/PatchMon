-- CreateTable (with existence check for 1.2.7 compatibility)
DO $$
BEGIN
    -- Check if table already exists (from 1.2.7 installation)
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.tables 
        WHERE table_schema = 'public' 
        AND table_name = 'user_sessions'
    ) THEN
        -- Table doesn't exist, create it
        CREATE TABLE "user_sessions" (
            "id" TEXT NOT NULL,
            "user_id" TEXT NOT NULL,
            "refresh_token" TEXT NOT NULL,
            "access_token_hash" TEXT,
            "ip_address" TEXT,
            "user_agent" TEXT,
            "last_activity" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
            "expires_at" TIMESTAMP(3) NOT NULL,
            "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
            "is_revoked" BOOLEAN NOT NULL DEFAULT false,

            CONSTRAINT "user_sessions_pkey" PRIMARY KEY ("id")
        );
        
        RAISE NOTICE 'Created user_sessions table';
    ELSE
        RAISE NOTICE 'user_sessions table already exists, skipping creation';
    END IF;
END $$;

-- CreateIndex (with existence check)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes 
        WHERE tablename = 'user_sessions' 
        AND indexname = 'user_sessions_refresh_token_key'
    ) THEN
        CREATE UNIQUE INDEX "user_sessions_refresh_token_key" ON "user_sessions"("refresh_token");
        RAISE NOTICE 'Created user_sessions_refresh_token_key index';
    ELSE
        RAISE NOTICE 'user_sessions_refresh_token_key index already exists, skipping';
    END IF;
END $$;

-- CreateIndex (with existence check)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes 
        WHERE tablename = 'user_sessions' 
        AND indexname = 'user_sessions_user_id_idx'
    ) THEN
        CREATE INDEX "user_sessions_user_id_idx" ON "user_sessions"("user_id");
        RAISE NOTICE 'Created user_sessions_user_id_idx index';
    ELSE
        RAISE NOTICE 'user_sessions_user_id_idx index already exists, skipping';
    END IF;
END $$;

-- CreateIndex (with existence check)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes 
        WHERE tablename = 'user_sessions' 
        AND indexname = 'user_sessions_refresh_token_idx'
    ) THEN
        CREATE INDEX "user_sessions_refresh_token_idx" ON "user_sessions"("refresh_token");
        RAISE NOTICE 'Created user_sessions_refresh_token_idx index';
    ELSE
        RAISE NOTICE 'user_sessions_refresh_token_idx index already exists, skipping';
    END IF;
END $$;

-- CreateIndex (with existence check)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_indexes 
        WHERE tablename = 'user_sessions' 
        AND indexname = 'user_sessions_expires_at_idx'
    ) THEN
        CREATE INDEX "user_sessions_expires_at_idx" ON "user_sessions"("expires_at");
        RAISE NOTICE 'Created user_sessions_expires_at_idx index';
    ELSE
        RAISE NOTICE 'user_sessions_expires_at_idx index already exists, skipping';
    END IF;
END $$;

-- AddForeignKey (with existence check)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints 
        WHERE table_name = 'user_sessions' 
        AND constraint_name = 'user_sessions_user_id_fkey'
    ) THEN
        ALTER TABLE "user_sessions" ADD CONSTRAINT "user_sessions_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE ON UPDATE CASCADE;
        RAISE NOTICE 'Created user_sessions_user_id_fkey foreign key';
    ELSE
        RAISE NOTICE 'user_sessions_user_id_fkey foreign key already exists, skipping';
    END IF;
END $$;

