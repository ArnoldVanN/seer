package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type Dashboard struct {
	ent.Schema
}

func (Dashboard) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").Unique().NotEmpty(),
		field.String("title"),
		field.Enum("visibility").Values("public", "private"),
		field.Int("position"),
		field.Strings("groups").Optional(),
		field.Strings("endpoint_keys").Optional(),
		field.Strings("allowed_groups").Optional().
			Comment("Private dashboards only: auth groups allowed to view it. Empty means any signed-in user."),
		field.String("site").Default("").
			Comment("Key of the site that defines it, empty at the top level. Its groups match only endpoints of the same site."),
		field.String("description").Default(""),
		field.String("default_range").Default("").
			Comment("History range shown when the URL doesn't pick one: 24h, 7d, 30d, 90d or 1y."),
		field.Text("custom_css").Default(""),
		field.Enum("source").Values("config", "api").Default("config").
			Comment("What owns the row: config sync only touches config rows, the write API only api rows."),
	}
}
