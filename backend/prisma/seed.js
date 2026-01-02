const { seedComplianceProfiles } = require("./seeds/compliance-profiles");

async function main() {
  await seedComplianceProfiles();
}

main()
  .catch((e) => {
    console.error(e);
    process.exit(1);
  });
