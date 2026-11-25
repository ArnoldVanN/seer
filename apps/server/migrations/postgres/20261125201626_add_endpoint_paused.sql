-- Modify "endpoints" table
ALTER TABLE "endpoints" ADD COLUMN "paused" boolean NOT NULL DEFAULT false, ADD COLUMN "resumed_at" timestamptz NULL;
