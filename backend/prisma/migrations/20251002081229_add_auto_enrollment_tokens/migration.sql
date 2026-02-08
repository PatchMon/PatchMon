-- CreateTable
CREATE TABLE "auto_enrollment_tokens" (
    "id" TEXT NOT NULL,
    "token_name" TEXT NOT NULL,
    "token_key" TEXT NOT NULL,
    "token_secret" TEXT NOT NULL,
    "created_by_user_id" TEXT,
    "is_active" BOOLEAN NOT NULL DEFAULT true,
    "allowed_ip_ranges" TEXT[],
    "max_hosts_per_day" INTEGER NOT NULL DEFAULT 100,
    "hosts_created_today" INTEGER NOT NULL DEFAULT 0,
    "last_reset_date" DATE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "default_host_group_id" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,
    "last_used_at" TIMESTAMP(3),
    "expires_at" TIMESTAMP(3),
    "metadata" JSONB,

    CONSTRAINT "auto_enrollment_tokens_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "auto_enrollment_tokens_token_key_key" ON "auto_enrollment_tokens"("token_key");

-- CreateIndex
CREATE INDEX "auto_enrollment_tokens_token_key_idx" ON "auto_enrollment_tokens"("token_key");

-- CreateIndex
CREATE INDEX "auto_enrollment_tokens_is_active_idx" ON "auto_enrollment_tokens"("is_active");

-- AddForeignKey
ALTER TABLE "auto_enrollment_tokens" ADD CONSTRAINT "auto_enrollment_tokens_created_by_user_id_fkey" FOREIGN KEY ("created_by_user_id") REFERENCES "users"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "auto_enrollment_tokens" ADD CONSTRAINT "auto_enrollment_tokens_default_host_group_id_fkey" FOREIGN KEY ("default_host_group_id") REFERENCES "host_groups"("id") ON DELETE SET NULL ON UPDATE CASCADE;

