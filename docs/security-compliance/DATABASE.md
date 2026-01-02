# Security Compliance Database Schema

## Overview

This document defines the database schema for the Security Compliance feature. The schema follows existing PatchMon patterns using PostgreSQL with Prisma ORM.

## Entity Relationship Diagram

```
┌─────────────────────┐       ┌─────────────────────┐
│  compliance_profiles│       │        hosts        │
├─────────────────────┤       ├─────────────────────┤
│ id (PK)             │       │ id (PK)             │
│ name                │       │ ...                 │
│ type                │       └──────────┬──────────┘
│ os_family           │                  │
│ version             │                  │
│ description         │                  │
│ created_at          │                  │
│ updated_at          │                  │
└──────────┬──────────┘                  │
           │                             │
           │ 1                           │ 1
           │                             │
           ▼ *                           ▼ *
┌─────────────────────┐       ┌─────────────────────┐
│  compliance_rules   │       │  compliance_scans   │
├─────────────────────┤       ├─────────────────────┤
│ id (PK)             │       │ id (PK)             │
│ profile_id (FK)     │◄──────│ profile_id (FK)     │
│ rule_ref            │       │ host_id (FK)        │────────┐
│ title               │       │ started_at          │        │
│ description         │       │ completed_at        │        │
│ rationale           │       │ status              │        │
│ severity            │       │ total_rules         │        │
│ section             │       │ passed              │        │
│ remediation         │       │ failed              │        │
└──────────┬──────────┘       │ warnings            │        │
           │                  │ skipped             │        │
           │ 1                │ score               │        │
           │                  │ raw_output          │        │
           │                  │ created_at          │        │
           ▼ *                └──────────┬──────────┘        │
┌─────────────────────┐                  │                   │
│ compliance_results  │                  │ 1                 │
├─────────────────────┤                  │                   │
│ id (PK)             │                  │                   │
│ scan_id (FK)        │◄─────────────────┘                   │
│ rule_id (FK)        │                                      │
│ status              │                                      │
│ finding             │                                      │
│ actual              │                                      │
│ expected            │                                      │
│ remediation         │                                      │
│ created_at          │                                      │
└─────────────────────┘                                      │
                                                             │
                        ◄────────────────────────────────────┘
```

## Table Definitions

### compliance_profiles

Stores predefined compliance profile definitions (CIS benchmarks, etc.).

```prisma
model compliance_profiles {
  id          String   @id @default(uuid())
  name        String   @unique
  type        String   // "openscap" or "docker-bench"
  os_family   String?  // "ubuntu", "rhel", "debian", null for docker
  version     String?  // Profile version, e.g., "1.0.0"
  description String?
  created_at  DateTime @default(now())
  updated_at  DateTime @updatedAt

  compliance_scans  compliance_scans[]
  compliance_rules  compliance_rules[]

  @@index([type])
  @@index([os_family])
}
```

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PK | Primary key |
| `name` | String | Unique, Not Null | Profile display name (e.g., "CIS Ubuntu 22.04 L1") |
| `type` | String | Not Null | Scanner type: "openscap" or "docker-bench" |
| `os_family` | String | Nullable | Target OS: "ubuntu", "rhel", "debian", null for docker |
| `version` | String | Nullable | Profile version string |
| `description` | Text | Nullable | Long-form profile description |
| `created_at` | DateTime | Default now() | Record creation timestamp |
| `updated_at` | DateTime | Auto-update | Last modification timestamp |

### compliance_scans

Records individual scan executions with summary statistics.

```prisma
model compliance_scans {
  id           String    @id @default(uuid())
  host_id      String
  profile_id   String
  started_at   DateTime
  completed_at DateTime?
  status       String    // "running", "completed", "failed"
  total_rules  Int       @default(0)
  passed       Int       @default(0)
  failed       Int       @default(0)
  warnings     Int       @default(0)
  skipped      Int       @default(0)
  score        Float?    // Percentage 0-100
  raw_output   String?   @db.Text
  created_at   DateTime  @default(now())

  hosts               hosts                @relation(fields: [host_id], references: [id], onDelete: Cascade)
  compliance_profiles compliance_profiles  @relation(fields: [profile_id], references: [id], onDelete: Cascade)
  compliance_results  compliance_results[]

  @@index([host_id])
  @@index([profile_id])
  @@index([status])
  @@index([started_at])
  @@index([host_id, started_at])
  @@index([host_id, profile_id])
}
```

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PK | Primary key |
| `host_id` | UUID | FK → hosts.id, Not Null | Target host |
| `profile_id` | UUID | FK → compliance_profiles.id, Not Null | Profile used |
| `started_at` | DateTime | Not Null | Scan start time |
| `completed_at` | DateTime | Nullable | Scan completion time |
| `status` | String | Not Null | Scan status: "running", "completed", "failed" |
| `total_rules` | Int | Default 0 | Total rules evaluated |
| `passed` | Int | Default 0 | Rules that passed |
| `failed` | Int | Default 0 | Rules that failed |
| `warnings` | Int | Default 0 | Rules with warnings |
| `skipped` | Int | Default 0 | Rules that were skipped |
| `score` | Float | Nullable | Compliance score percentage (0-100) |
| `raw_output` | Text | Nullable | Full scanner output for debugging |
| `created_at` | DateTime | Default now() | Record creation timestamp |

### compliance_rules

Stores rule definitions from profiles for reference and display.

```prisma
model compliance_rules {
  id          String  @id @default(uuid())
  profile_id  String
  rule_ref    String  // Original rule ID from CIS/OpenSCAP
  title       String
  description String? @db.Text
  rationale   String? @db.Text
  severity    String? // "low", "medium", "high", "critical"
  section     String? // CIS section number (e.g., "1.1.1")
  remediation String? @db.Text

  compliance_profiles compliance_profiles  @relation(fields: [profile_id], references: [id], onDelete: Cascade)
  compliance_results  compliance_results[]

  @@unique([profile_id, rule_ref])
  @@index([profile_id])
  @@index([severity])
  @@index([section])
}
```

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PK | Primary key |
| `profile_id` | UUID | FK → compliance_profiles.id, Not Null | Parent profile |
| `rule_ref` | String | Not Null | Original rule ID from scanner |
| `title` | String | Not Null | Rule title/summary |
| `description` | Text | Nullable | Detailed rule description |
| `rationale` | Text | Nullable | Why this rule matters |
| `severity` | String | Nullable | Impact level: "low", "medium", "high", "critical" |
| `section` | String | Nullable | CIS section number |
| `remediation` | Text | Nullable | How to fix violations |

**Constraints:**
- Unique on `(profile_id, rule_ref)` - each rule appears once per profile

### compliance_results

Individual rule evaluation results from a scan.

```prisma
model compliance_results {
  id          String   @id @default(uuid())
  scan_id     String
  rule_id     String
  status      String   // "pass", "fail", "warn", "skip", "notapplicable", "error"
  finding     String?  @db.Text
  actual      String?  @db.Text
  expected    String?  @db.Text
  remediation String?  @db.Text
  created_at  DateTime @default(now())

  compliance_scans compliance_scans @relation(fields: [scan_id], references: [id], onDelete: Cascade)
  compliance_rules compliance_rules @relation(fields: [rule_id], references: [id], onDelete: Cascade)

  @@index([scan_id])
  @@index([rule_id])
  @@index([status])
  @@index([scan_id, status])
}
```

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PK | Primary key |
| `scan_id` | UUID | FK → compliance_scans.id, Not Null | Parent scan |
| `rule_id` | UUID | FK → compliance_rules.id, Not Null | Rule evaluated |
| `status` | String | Not Null | Result: "pass", "fail", "warn", "skip", "notapplicable", "error" |
| `finding` | Text | Nullable | What was found during evaluation |
| `actual` | Text | Nullable | Actual value observed |
| `expected` | Text | Nullable | Expected value per benchmark |
| `remediation` | Text | Nullable | Specific remediation guidance |
| `created_at` | DateTime | Default now() | Record creation timestamp |

## Prisma Schema Addition

Add to `backend/prisma/schema.prisma`:

```prisma
model compliance_profiles {
  id          String   @id @default(uuid())
  name        String   @unique
  type        String
  os_family   String?
  version     String?
  description String?
  created_at  DateTime @default(now())
  updated_at  DateTime @updatedAt

  compliance_scans compliance_scans[]
  compliance_rules compliance_rules[]

  @@index([type])
  @@index([os_family])
}

model compliance_scans {
  id           String    @id @default(uuid())
  host_id      String
  profile_id   String
  started_at   DateTime
  completed_at DateTime?
  status       String
  total_rules  Int       @default(0)
  passed       Int       @default(0)
  failed       Int       @default(0)
  warnings     Int       @default(0)
  skipped      Int       @default(0)
  score        Float?
  raw_output   String?   @db.Text
  created_at   DateTime  @default(now())

  hosts               hosts               @relation(fields: [host_id], references: [id], onDelete: Cascade)
  compliance_profiles compliance_profiles @relation(fields: [profile_id], references: [id], onDelete: Cascade)
  compliance_results  compliance_results[]

  @@index([host_id])
  @@index([profile_id])
  @@index([status])
  @@index([started_at])
  @@index([host_id, started_at])
  @@index([host_id, profile_id])
}

model compliance_rules {
  id          String  @id @default(uuid())
  profile_id  String
  rule_ref    String
  title       String
  description String? @db.Text
  rationale   String? @db.Text
  severity    String?
  section     String?
  remediation String? @db.Text

  compliance_profiles compliance_profiles  @relation(fields: [profile_id], references: [id], onDelete: Cascade)
  compliance_results  compliance_results[]

  @@unique([profile_id, rule_ref])
  @@index([profile_id])
  @@index([severity])
  @@index([section])
}

model compliance_results {
  id          String   @id @default(uuid())
  scan_id     String
  rule_id     String
  status      String
  finding     String?  @db.Text
  actual      String?  @db.Text
  expected    String?  @db.Text
  remediation String?  @db.Text
  created_at  DateTime @default(now())

  compliance_scans compliance_scans @relation(fields: [scan_id], references: [id], onDelete: Cascade)
  compliance_rules compliance_rules @relation(fields: [rule_id], references: [id], onDelete: Cascade)

  @@index([scan_id])
  @@index([rule_id])
  @@index([status])
  @@index([scan_id, status])
}
```

## Hosts Table Update

Add relation to hosts model:

```prisma
model hosts {
  // ... existing fields ...
  compliance_scans compliance_scans[]
}
```

## Seed Data

Initial compliance profiles to seed:

```javascript
// prisma/seed-compliance.js
const profiles = [
  {
    name: "CIS Ubuntu 22.04 L1",
    type: "openscap",
    os_family: "ubuntu",
    version: "1.0.0",
    description: "CIS Benchmark for Ubuntu 22.04 LTS - Level 1 Server"
  },
  {
    name: "CIS Ubuntu 22.04 L2",
    type: "openscap",
    os_family: "ubuntu",
    version: "1.0.0",
    description: "CIS Benchmark for Ubuntu 22.04 LTS - Level 2 Server"
  },
  {
    name: "CIS Ubuntu 20.04 L1",
    type: "openscap",
    os_family: "ubuntu",
    version: "1.1.0",
    description: "CIS Benchmark for Ubuntu 20.04 LTS - Level 1 Server"
  },
  {
    name: "CIS RHEL 8 L1",
    type: "openscap",
    os_family: "rhel",
    version: "2.0.0",
    description: "CIS Benchmark for Red Hat Enterprise Linux 8 - Level 1 Server"
  },
  {
    name: "CIS RHEL 8 L2",
    type: "openscap",
    os_family: "rhel",
    version: "2.0.0",
    description: "CIS Benchmark for Red Hat Enterprise Linux 8 - Level 2 Server"
  },
  {
    name: "CIS Debian 11 L1",
    type: "openscap",
    os_family: "debian",
    version: "1.0.0",
    description: "CIS Benchmark for Debian 11 - Level 1 Server"
  },
  {
    name: "CIS Docker",
    type: "docker-bench",
    os_family: null,
    version: "1.5.0",
    description: "CIS Docker Benchmark v1.5.0"
  }
];
```

## Migration

Create migration file `prisma/migrations/YYYYMMDDHHMMSS_add_compliance_tables/migration.sql`:

```sql
-- CreateTable
CREATE TABLE "compliance_profiles" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "type" TEXT NOT NULL,
    "os_family" TEXT,
    "version" TEXT,
    "description" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "compliance_profiles_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "compliance_scans" (
    "id" TEXT NOT NULL,
    "host_id" TEXT NOT NULL,
    "profile_id" TEXT NOT NULL,
    "started_at" TIMESTAMP(3) NOT NULL,
    "completed_at" TIMESTAMP(3),
    "status" TEXT NOT NULL,
    "total_rules" INTEGER NOT NULL DEFAULT 0,
    "passed" INTEGER NOT NULL DEFAULT 0,
    "failed" INTEGER NOT NULL DEFAULT 0,
    "warnings" INTEGER NOT NULL DEFAULT 0,
    "skipped" INTEGER NOT NULL DEFAULT 0,
    "score" DOUBLE PRECISION,
    "raw_output" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "compliance_scans_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "compliance_rules" (
    "id" TEXT NOT NULL,
    "profile_id" TEXT NOT NULL,
    "rule_ref" TEXT NOT NULL,
    "title" TEXT NOT NULL,
    "description" TEXT,
    "rationale" TEXT,
    "severity" TEXT,
    "section" TEXT,
    "remediation" TEXT,

    CONSTRAINT "compliance_rules_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "compliance_results" (
    "id" TEXT NOT NULL,
    "scan_id" TEXT NOT NULL,
    "rule_id" TEXT NOT NULL,
    "status" TEXT NOT NULL,
    "finding" TEXT,
    "actual" TEXT,
    "expected" TEXT,
    "remediation" TEXT,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "compliance_results_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "compliance_profiles_name_key" ON "compliance_profiles"("name");
CREATE INDEX "compliance_profiles_type_idx" ON "compliance_profiles"("type");
CREATE INDEX "compliance_profiles_os_family_idx" ON "compliance_profiles"("os_family");

-- CreateIndex
CREATE INDEX "compliance_scans_host_id_idx" ON "compliance_scans"("host_id");
CREATE INDEX "compliance_scans_profile_id_idx" ON "compliance_scans"("profile_id");
CREATE INDEX "compliance_scans_status_idx" ON "compliance_scans"("status");
CREATE INDEX "compliance_scans_started_at_idx" ON "compliance_scans"("started_at");
CREATE INDEX "compliance_scans_host_id_started_at_idx" ON "compliance_scans"("host_id", "started_at");
CREATE INDEX "compliance_scans_host_id_profile_id_idx" ON "compliance_scans"("host_id", "profile_id");

-- CreateIndex
CREATE UNIQUE INDEX "compliance_rules_profile_id_rule_ref_key" ON "compliance_rules"("profile_id", "rule_ref");
CREATE INDEX "compliance_rules_profile_id_idx" ON "compliance_rules"("profile_id");
CREATE INDEX "compliance_rules_severity_idx" ON "compliance_rules"("severity");
CREATE INDEX "compliance_rules_section_idx" ON "compliance_rules"("section");

-- CreateIndex
CREATE INDEX "compliance_results_scan_id_idx" ON "compliance_results"("scan_id");
CREATE INDEX "compliance_results_rule_id_idx" ON "compliance_results"("rule_id");
CREATE INDEX "compliance_results_status_idx" ON "compliance_results"("status");
CREATE INDEX "compliance_results_scan_id_status_idx" ON "compliance_results"("scan_id", "status");

-- AddForeignKey
ALTER TABLE "compliance_scans" ADD CONSTRAINT "compliance_scans_host_id_fkey"
    FOREIGN KEY ("host_id") REFERENCES "hosts"("id") ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE "compliance_scans" ADD CONSTRAINT "compliance_scans_profile_id_fkey"
    FOREIGN KEY ("profile_id") REFERENCES "compliance_profiles"("id") ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE "compliance_rules" ADD CONSTRAINT "compliance_rules_profile_id_fkey"
    FOREIGN KEY ("profile_id") REFERENCES "compliance_profiles"("id") ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE "compliance_results" ADD CONSTRAINT "compliance_results_scan_id_fkey"
    FOREIGN KEY ("scan_id") REFERENCES "compliance_scans"("id") ON DELETE CASCADE ON UPDATE CASCADE;

ALTER TABLE "compliance_results" ADD CONSTRAINT "compliance_results_rule_id_fkey"
    FOREIGN KEY ("rule_id") REFERENCES "compliance_rules"("id") ON DELETE CASCADE ON UPDATE CASCADE;
```

## Query Examples

### Get latest scan for a host

```javascript
const latestScan = await prisma.compliance_scans.findFirst({
  where: { host_id: hostId },
  orderBy: { started_at: 'desc' },
  include: {
    compliance_profiles: true,
    compliance_results: {
      include: {
        compliance_rules: true
      }
    }
  }
});
```

### Get dashboard statistics

```javascript
const stats = await prisma.$queryRaw`
  SELECT
    COUNT(DISTINCT cs.host_id) as hosts_scanned,
    AVG(cs.score) as avg_score,
    SUM(CASE WHEN cs.score >= 80 THEN 1 ELSE 0 END) as compliant_hosts,
    SUM(CASE WHEN cs.score < 80 AND cs.score >= 50 THEN 1 ELSE 0 END) as warning_hosts,
    SUM(CASE WHEN cs.score < 50 THEN 1 ELSE 0 END) as critical_hosts
  FROM compliance_scans cs
  INNER JOIN (
    SELECT host_id, MAX(started_at) as max_started
    FROM compliance_scans
    WHERE status = 'completed'
    GROUP BY host_id
  ) latest ON cs.host_id = latest.host_id AND cs.started_at = latest.max_started
`;
```

### Get top failing rules

```javascript
const topFailingRules = await prisma.compliance_results.groupBy({
  by: ['rule_id'],
  where: { status: 'fail' },
  _count: { rule_id: true },
  orderBy: { _count: { rule_id: 'desc' } },
  take: 10
});
```
