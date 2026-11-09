-- Modify "dashboards" table
ALTER TABLE "dashboards" ADD COLUMN "source" character varying NOT NULL DEFAULT 'config';
-- Modify "endpoints" table
ALTER TABLE "endpoints" ADD COLUMN "source" character varying NOT NULL DEFAULT 'config';
