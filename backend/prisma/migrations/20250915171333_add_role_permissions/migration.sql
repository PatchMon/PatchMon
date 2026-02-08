/*
  Warnings:

  - You are about to drop the column `token` on the `hosts` table. All the data in the column will be lost.
  - A unique constraint covering the columns `[api_id]` on the table `hosts` will be added. If there are existing duplicate values, this will fail.
  - A unique constraint covering the columns `[api_key]` on the table `hosts` will be added. If there are existing duplicate values, this will fail.
  - Added the required column `api_id` to the `hosts` table without a default value. This is not possible if the table is not empty.
  - Added the required column `api_key` to the `hosts` table without a default value. This is not possible if the table is not empty.

*/
-- DropIndex
DROP INDEX "hosts_token_key";

-- AlterTable
ALTER TABLE "hosts" DROP COLUMN "token",
ADD COLUMN     "api_id" TEXT NOT NULL,
ADD COLUMN     "api_key" TEXT NOT NULL;

-- CreateTable
CREATE TABLE "users" (
    "id" TEXT NOT NULL,
    "username" TEXT NOT NULL,
    "email" TEXT NOT NULL,
    "password_hash" TEXT NOT NULL,
    "role" TEXT NOT NULL DEFAULT 'admin',
    "is_active" BOOLEAN NOT NULL DEFAULT true,
    "last_login" TIMESTAMP(3),
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "users_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "role_permissions" (
    "id" TEXT NOT NULL,
    "role" TEXT NOT NULL,
    "can_view_dashboard" BOOLEAN NOT NULL DEFAULT true,
    "can_view_hosts" BOOLEAN NOT NULL DEFAULT true,
    "can_manage_hosts" BOOLEAN NOT NULL DEFAULT false,
    "can_view_packages" BOOLEAN NOT NULL DEFAULT true,
    "can_manage_packages" BOOLEAN NOT NULL DEFAULT false,
    "can_view_users" BOOLEAN NOT NULL DEFAULT false,
    "can_manage_users" BOOLEAN NOT NULL DEFAULT false,
    "can_view_reports" BOOLEAN NOT NULL DEFAULT true,
    "can_export_data" BOOLEAN NOT NULL DEFAULT false,
    "can_manage_settings" BOOLEAN NOT NULL DEFAULT false,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "role_permissions_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "users_username_key" ON "users"("username");

-- CreateIndex
CREATE UNIQUE INDEX "users_email_key" ON "users"("email");

-- CreateIndex
CREATE UNIQUE INDEX "role_permissions_role_key" ON "role_permissions"("role");

-- CreateIndex
CREATE UNIQUE INDEX "hosts_api_id_key" ON "hosts"("api_id");

-- CreateIndex
CREATE UNIQUE INDEX "hosts_api_key_key" ON "hosts"("api_key");
