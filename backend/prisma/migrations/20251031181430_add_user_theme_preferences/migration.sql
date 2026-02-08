-- AlterTable
ALTER TABLE "users" ADD COLUMN IF NOT EXISTS "theme_preference" VARCHAR(10) DEFAULT 'dark';

-- AlterTable
ALTER TABLE "users" ADD COLUMN IF NOT EXISTS "color_theme" VARCHAR(50) DEFAULT 'cyber_blue';

