# For inspecting the schema with the atlas CLI. New migrations are generated
# with scripts/migrate-diff.sh, since the community CLI rejects the ent provider.
data "external_schema" "ent" {
  program = [
    "go", "run", "-mod=mod",
    "ariga.io/atlas-provider-ent/cmd/atlas-provider-ent",
    "--path", "./ent/schema",
    "--dialect", "postgres",
  ]
}

env "local" {
  src = data.external_schema.ent.url
  dev = "docker://postgres/18/dev?search_path=public"
  migration {
    dir = "file://migrations/postgres"
  }
}
