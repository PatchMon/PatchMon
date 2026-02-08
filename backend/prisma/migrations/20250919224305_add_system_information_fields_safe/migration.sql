-- AlterTable
ALTER TABLE "hosts" ADD COLUMN     "cpu_cores" INTEGER,
ADD COLUMN     "cpu_model" TEXT,
ADD COLUMN     "disk_details" JSONB,
ADD COLUMN     "dns_servers" JSONB,
ADD COLUMN     "gateway_ip" TEXT,
ADD COLUMN     "hostname" TEXT,
ADD COLUMN     "kernel_version" TEXT,
ADD COLUMN     "load_average" JSONB,
ADD COLUMN     "network_interfaces" JSONB,
ADD COLUMN     "ram_installed" INTEGER,
ADD COLUMN     "selinux_status" TEXT,
ADD COLUMN     "swap_size" INTEGER,
ADD COLUMN     "system_uptime" TEXT;
