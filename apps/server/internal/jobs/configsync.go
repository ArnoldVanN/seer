// Package jobs holds the cluster-wide singleton jobs. Each runs under its own
// advisory lock, so any replica can run it and only one does at a time.
package jobs

import (
	"context"
	stdsql "database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"entgo.io/ent/dialect/sql"
	"github.com/rs/zerolog/log"

	"github.com/ArnoldVanN/seer/apps/server/ent"
	"github.com/ArnoldVanN/seer/apps/server/ent/dashboard"
	"github.com/ArnoldVanN/seer/apps/server/ent/endpoint"
	"github.com/ArnoldVanN/seer/apps/server/ent/site"
	"github.com/ArnoldVanN/seer/apps/server/internal/config"
	"github.com/ArnoldVanN/seer/apps/server/internal/db"
)

// SyncConfig writes cfg's sites, endpoints and dashboards to the DB, which is what
// workers and the API read, so all replicas agree even mid-rollout.
//
// generation orders configs (the Helm chart passes the release revision).
// A config with a higher generation wins; an equal generation with a different
// hash also wins, so without generations the most recently started replica's
// config applies. Older replicas never overwrite a newer config.
func SyncConfig(ctx context.Context, d *db.DB, cfg *config.Config, generation int64, replicaID string) error {
	return d.WithLock(ctx, db.LockConfigSync, func(ctx context.Context) error {
		cur, err := d.Ent.ConfigVersion.Query().Order(ent.Desc("id")).First(ctx)
		if err != nil && !ent.IsNotFound(err) {
			return err
		}
		if cur != nil {
			switch {
			case cur.Hash == cfg.Hash:
				log.Info().Str("hash", short(cfg.Hash)).Msg("config already synced")
				return nil
			case generation < cur.Generation:
				log.Warn().Int64("ours", generation).Int64("synced", cur.Generation).
					Msg("a newer config generation is already synced; not applying ours")
				return nil
			}
		}

		tx, err := d.Ent.Tx(ctx)
		if err != nil {
			return err
		}
		if err := apply(ctx, tx, cfg); err != nil {
			tx.Rollback() //nolint:errcheck
			return err
		}
		if err := tx.ConfigVersion.Create().
			SetHash(cfg.Hash).SetGeneration(generation).SetAppliedBy(replicaID).
			Exec(ctx); err != nil {
			tx.Rollback() //nolint:errcheck
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		log.Info().Str("hash", short(cfg.Hash)).Int64("generation", generation).
			Int("sites", len(cfg.Sites)).Int("endpoints", len(cfg.Endpoints)).Int("dashboards", len(cfg.Dashboards)).
			Msg("config synced")
		return nil
	})
}

func apply(ctx context.Context, tx *ent.Tx, cfg *config.Config) error {
	// Every upsert only overwrites a config row. On a key the write API owns
	// it updates nothing, and ent reports that as sql.ErrNoRows.
	keys := make([]string, 0, len(cfg.Endpoints))
	for _, e := range cfg.Endpoints {
		keys = append(keys, e.Key)
		spec, err := json.Marshal(e)
		if err != nil {
			return err
		}
		err = tx.Endpoint.Create().
			SetKey(e.Key).
			SetName(e.Name).
			SetGroupName(e.Group).
			SetType(e.Type).
			SetIntervalSeconds(max(1, int(e.Interval.D().Seconds()))).
			SetEnabled(e.IsEnabled()).
			SetSpec(spec).
			SetSite(e.Site).
			SetSource(endpoint.SourceConfig).
			OnConflict(
				sql.ConflictColumns(endpoint.FieldKey),
				sql.ResolveWithNewValues(),
				sql.UpdateWhere(sql.EQ(endpoint.FieldSource, endpoint.SourceConfig.String())),
			).
			Exec(ctx)
		if errors.Is(err, stdsql.ErrNoRows) {
			log.Warn().Str("endpoint", e.Key).Msg("endpoint key is owned by the API; skipping the config's version")
			continue
		}
		if err != nil {
			return fmt.Errorf("upsert endpoint %s: %w", e.Key, err)
		}
	}
	// Endpoints removed from the config are disabled rather than deleted so
	// their history survives a typo or a temporary removal.
	if _, err := tx.Endpoint.Update().
		Where(endpoint.SourceEQ(endpoint.SourceConfig), endpoint.KeyNotIn(keys...)).
		SetEnabled(false).
		Save(ctx); err != nil {
		return err
	}

	dkeys := make([]string, 0, len(cfg.Dashboards))
	for i, dsh := range cfg.Dashboards {
		dkeys = append(dkeys, dsh.Key)
		err := tx.Dashboard.Create().
			SetKey(dsh.Key).
			SetTitle(dsh.Title).
			SetVisibility(dashboard.Visibility(dsh.Visibility)).
			SetPosition(i).
			SetGroups(dsh.Groups).
			SetEndpointKeys(dsh.Endpoints).
			SetAllowedGroups(dsh.AllowedGroups).
			SetSite(dsh.Site).
			SetDescription(dsh.Description).
			SetDefaultRange(dsh.DefaultRange).
			SetCustomCSS(dsh.CustomCSS).
			SetSource(dashboard.SourceConfig).
			OnConflict(
				sql.ConflictColumns(dashboard.FieldKey),
				sql.ResolveWithNewValues(),
				sql.UpdateWhere(sql.EQ(dashboard.FieldSource, dashboard.SourceConfig.String())),
			).
			Exec(ctx)
		if errors.Is(err, stdsql.ErrNoRows) {
			log.Warn().Str("dashboard", dsh.Key).Msg("dashboard key is owned by the API; skipping the config's version")
			continue
		}
		if err != nil {
			return fmt.Errorf("upsert dashboard %s: %w", dsh.Key, err)
		}
	}
	if _, err := tx.Dashboard.Delete().
		Where(dashboard.SourceEQ(dashboard.SourceConfig), dashboard.KeyNotIn(dkeys...)).
		Exec(ctx); err != nil {
		return err
	}
	return applySites(ctx, tx, cfg)
}

func applySites(ctx context.Context, tx *ent.Tx, cfg *config.Config) error {
	skeys := make([]string, 0, len(cfg.Sites))
	for i, st := range cfg.Sites {
		skeys = append(skeys, st.Key)
		ui, err := json.Marshal(st.OwnUI)
		if err != nil {
			return err
		}
		dashboards := st.Dashboards
		if st.AllDashboards {
			dashboards = nil
		}
		err = tx.Site.Create().
			SetKey(st.Key).
			SetPosition(i).
			SetHosts(st.Hosts).
			SetDashboards(dashboards).
			SetAllDashboards(st.AllDashboards).
			SetPrivate(st.Private).
			SetAllowedGroups(st.AllowedGroups).
			SetUI(ui).
			SetSource(site.SourceConfig).
			OnConflict(
				sql.ConflictColumns(site.FieldKey),
				sql.ResolveWithNewValues(),
				sql.UpdateWhere(sql.EQ(site.FieldSource, site.SourceConfig.String())),
			).
			Exec(ctx)
		if errors.Is(err, stdsql.ErrNoRows) {
			log.Warn().Str("site", st.Key).Msg("site key is owned by the API; skipping the config's version")
			continue
		}
		if err != nil {
			return fmt.Errorf("upsert site %s: %w", st.Key, err)
		}
	}
	_, err := tx.Site.Delete().
		Where(site.SourceEQ(site.SourceConfig), site.KeyNotIn(skeys...)).
		Exec(ctx)
	return err
}

func short(h string) string { return h[:min(12, len(h))] }
