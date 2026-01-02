const { PrismaClient } = require("@prisma/client");

const prisma = new PrismaClient();

const defaultProfiles = [
  {
    name: "CIS Ubuntu 22.04 L1",
    type: "openscap",
    os_family: "ubuntu",
    version: "1.0.0",
    description: "CIS Benchmark for Ubuntu 22.04 LTS - Level 1 Server",
  },
  {
    name: "CIS Ubuntu 22.04 L2",
    type: "openscap",
    os_family: "ubuntu",
    version: "1.0.0",
    description: "CIS Benchmark for Ubuntu 22.04 LTS - Level 2 Server",
  },
  {
    name: "CIS Ubuntu 20.04 L1",
    type: "openscap",
    os_family: "ubuntu",
    version: "1.1.0",
    description: "CIS Benchmark for Ubuntu 20.04 LTS - Level 1 Server",
  },
  {
    name: "CIS RHEL 8 L1",
    type: "openscap",
    os_family: "rhel",
    version: "2.0.0",
    description: "CIS Benchmark for Red Hat Enterprise Linux 8 - Level 1 Server",
  },
  {
    name: "CIS RHEL 8 L2",
    type: "openscap",
    os_family: "rhel",
    version: "2.0.0",
    description: "CIS Benchmark for Red Hat Enterprise Linux 8 - Level 2 Server",
  },
  {
    name: "CIS Debian 11 L1",
    type: "openscap",
    os_family: "debian",
    version: "1.0.0",
    description: "CIS Benchmark for Debian 11 - Level 1 Server",
  },
  {
    name: "CIS Docker",
    type: "docker-bench",
    os_family: null,
    version: "1.5.0",
    description: "CIS Docker Benchmark v1.5.0",
  },
];

async function seedComplianceProfiles() {
  console.log("Seeding compliance profiles...");

  for (const profile of defaultProfiles) {
    await prisma.compliance_profiles.upsert({
      where: { name: profile.name },
      update: {
        type: profile.type,
        os_family: profile.os_family,
        version: profile.version,
        description: profile.description,
      },
      create: profile,
    });
    console.log(`  Created/updated profile: ${profile.name}`);
  }

  console.log("Compliance profiles seeded successfully!");
}

module.exports = { seedComplianceProfiles };

// Run directly if called as script
if (require.main === module) {
  seedComplianceProfiles()
    .catch((e) => {
      console.error(e);
      process.exit(1);
    })
    .finally(async () => {
      await prisma.$disconnect();
    });
}
