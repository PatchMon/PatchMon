-- AddLogoFieldsToSettings
ALTER TABLE "settings" ADD COLUMN "logo_dark" VARCHAR(255) DEFAULT '/assets/logo_dark.png';
ALTER TABLE "settings" ADD COLUMN "logo_light" VARCHAR(255) DEFAULT '/assets/logo_light.png';
ALTER TABLE "settings" ADD COLUMN "favicon" VARCHAR(255) DEFAULT '/assets/logo_square.svg';
