-- CreateTable
CREATE TABLE "hosts" (
    "id" TEXT NOT NULL,
    "hostname" TEXT NOT NULL,
    "ip" TEXT,
    "os_type" TEXT NOT NULL,
    "os_version" TEXT NOT NULL,
    "architecture" TEXT,
    "last_update" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "status" TEXT NOT NULL DEFAULT 'active',
    "token" TEXT NOT NULL,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "hosts_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "packages" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "description" TEXT,
    "category" TEXT,
    "latest_version" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "packages_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "host_packages" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "package_id" TEXT NOT NULL,
    "current_version" TEXT NOT NULL,
    "available_version" TEXT,
    "needs_update" BOOLEAN NOT NULL DEFAULT false,
    "is_security_update" BOOLEAN NOT NULL DEFAULT false,
    "last_checked" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "host_packages_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "update_history" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "packages_count" INTEGER NOT NULL,
    "security_count" INTEGER NOT NULL,
    "timestamp" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "status" TEXT NOT NULL DEFAULT 'success',
    "error_message" TEXT,

    CONSTRAINT "update_history_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "hosts_hostname_key" ON "hosts"("hostname");

-- CreateIndex
CREATE UNIQUE INDEX "hosts_token_key" ON "hosts"("token");

-- CreateIndex
CREATE UNIQUE INDEX "packages_name_key" ON "packages"("name");

-- CreateIndex
CREATE UNIQUE INDEX "host_packages_host_id_package_id_key" ON "host_packages"("host_id", "package_id");

-- AddForeignKey
ALTER TABLE "host_packages" ADD CONSTRAINT "host_packages_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "host_packages" ADD CONSTRAINT "host_packages_package_id_fkey" FOREIGN KEY ("package_id") REFERENCES "packages"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "update_history" ADD CONSTRAINT "update_history_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;
