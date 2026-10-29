package ent

//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert,sql/versioned-migration,sql/execquery,sql/modifier ./schema
