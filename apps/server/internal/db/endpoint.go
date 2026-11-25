package db

import (
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"

	"github.com/ArnoldVanN/seer/apps/server/ent/endpoint"
)

// StampResume is an endpoint upsert option that sets resumed_at when the
// write turns an inactive (disabled or paused) endpoint back on. A push
// watcher counts missed pings from it, so the time the endpoint was off
// isn't one long miss.
func StampResume() sql.ConflictOption {
	return sql.ResolveWith(func(u *sql.UpdateSet) {
		t := u.Table()
		ex := sql.Dialect(dialect.Postgres).Table("excluded")
		active := func(enabled, paused string) string {
			return "(" + enabled + " AND NOT " + paused + ")"
		}
		was := active(t.C(endpoint.FieldEnabled), t.C(endpoint.FieldPaused))
		now := active(ex.C(endpoint.FieldEnabled), ex.C(endpoint.FieldPaused))
		u.Set(endpoint.FieldResumedAt, sql.Expr(
			"CASE WHEN NOT "+was+" AND "+now+" THEN now() ELSE "+t.C(endpoint.FieldResumedAt)+" END",
		))
	})
}
