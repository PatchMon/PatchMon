-- AlterTable
ALTER TABLE "settings" ADD COLUMN "discord_allow_registration" BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE "settings" ADD COLUMN "discord_required_guild_id" TEXT;
