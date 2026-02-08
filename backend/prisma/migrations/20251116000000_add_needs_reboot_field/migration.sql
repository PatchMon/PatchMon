-- AlterTable
ALTER TABLE "hosts" ADD COLUMN "needs_reboot" BOOLEAN DEFAULT false;
ALTER TABLE "hosts" ADD COLUMN "installed_kernel_version" TEXT;

-- CreateIndex
CREATE INDEX "hosts_needs_reboot_idx" ON "hosts"("needs_reboot");

