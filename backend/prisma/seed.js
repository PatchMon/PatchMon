const { seedComplianceProfiles } = require("./seeds/compliance-profiles");
const { seedRolePermissions } = require("./seeds/role-permissions");

async function main() {
  await seedRolePermissions();
  await seedComplianceProfiles();
}

main()
  .catch((e) => {
    console.error(e);
    process.exit(1);
  });
