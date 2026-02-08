-- CreateTable
CREATE TABLE "repositories" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "url" TEXT NOT NULL,
    "distribution" TEXT NOT NULL,
    "components" TEXT NOT NULL,
    "repo_type" TEXT NOT NULL,
    "is_active" BOOLEAN NOT NULL DEFAULT true,
    "is_secure" BOOLEAN NOT NULL DEFAULT true,
    "priority" INTEGER,
    "description" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "repositories_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "host_repositories" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "repository_id" TEXT NOT NULL,
    "is_enabled" BOOLEAN NOT NULL DEFAULT true,
    "last_checked" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "host_repositories_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "repositories_url_distribution_components_key" ON "repositories"("url", "distribution", "components");

-- CreateIndex
CREATE UNIQUE INDEX "host_repositories_host_id_repository_id_key" ON "host_repositories"("host_id", "repository_id");

-- AddForeignKey
ALTER TABLE "host_repositories" ADD CONSTRAINT "host_repositories_host_id_fkey" FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "host_repositories" ADD CONSTRAINT "host_repositories_repository_id_fkey" FOREIGN KEY ("repository_id") REFERENCES "repositories"("id") ON DELETE CASCADE ON UPDATE CASCADE;
