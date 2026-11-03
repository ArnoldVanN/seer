-- Create "sessions" table
CREATE TABLE "sessions" ("id" character varying NOT NULL, "subject" character varying NOT NULL, "name" character varying NOT NULL, "email" character varying NOT NULL DEFAULT '', "username" character varying NOT NULL DEFAULT '', "groups" jsonb NULL, "user_agent" character varying NOT NULL DEFAULT '', "ip" character varying NOT NULL DEFAULT '', "created_at" timestamptz NOT NULL, "last_seen_at" timestamptz NOT NULL, "expires_at" timestamptz NOT NULL, "revoked_at" timestamptz NULL, PRIMARY KEY ("id"));
-- Create index "session_expires_at" to table: "sessions"
CREATE INDEX "session_expires_at" ON "sessions" ("expires_at");
-- Create index "session_subject" to table: "sessions"
CREATE INDEX "session_subject" ON "sessions" ("subject");
