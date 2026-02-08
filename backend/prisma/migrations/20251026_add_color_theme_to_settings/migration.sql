-- AlterTable
-- Add color_theme field to settings table for customizable app theming
ALTER TABLE "settings" ADD COLUMN "color_theme" TEXT NOT NULL DEFAULT 'default';

