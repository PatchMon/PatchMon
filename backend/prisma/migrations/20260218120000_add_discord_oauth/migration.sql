-- AlterTable
ALTER TABLE "users" ADD COLUMN "discord_id" TEXT,
ADD COLUMN "discord_username" TEXT,
ADD COLUMN "discord_avatar" TEXT,
ADD COLUMN "discord_linked_at" TIMESTAMP(3);

-- CreateIndex
CREATE UNIQUE INDEX "users_discord_id_key" ON "users"("discord_id");

-- AlterTable
ALTER TABLE "settings" ADD COLUMN "discord_oauth_enabled" BOOLEAN NOT NULL DEFAULT false,
ADD COLUMN "discord_client_id" TEXT,
ADD COLUMN "discord_client_secret" TEXT,
ADD COLUMN "discord_redirect_uri" TEXT,
ADD COLUMN "discord_button_text" TEXT DEFAULT 'Login with Discord';
