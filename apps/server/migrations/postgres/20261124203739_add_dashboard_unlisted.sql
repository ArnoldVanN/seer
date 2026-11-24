-- Modify "dashboards" table
ALTER TABLE "dashboards" ADD COLUMN "unlisted" boolean NOT NULL DEFAULT false;
