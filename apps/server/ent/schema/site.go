package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

// Site is a status page on its own hostnames. Requests for a host no site
// lists go to the site with the lowest position.
type Site struct {
	ent.Schema
}

func (Site) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").Unique().NotEmpty(),
		field.Int("position"),
		field.Strings("hosts").Optional(),
		field.Strings("dashboards").Optional().
			Comment("Dashboard keys in tab order. Ignored when all_dashboards is set."),
		field.Bool("all_dashboards").Default(false).
			Comment(`Lists every dashboard, as dashboards: ["*"] does in the config.`),
		field.Bool("private").Default(false),
		field.Strings("allowed_groups").Optional(),
		field.Bytes("ui").Comment("JSON-encoded config.UI: the site's own overrides, merged over the top-level ui when read."),
		field.Enum("source").Values("config", "api").Default("config").
			Comment("What owns the row: config sync only touches config rows, the write API only api rows."),
	}
}
