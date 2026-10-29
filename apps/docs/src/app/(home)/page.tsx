import Link from 'next/link';
import { Activity, Boxes, LayoutDashboard, Palette, Server, ShieldCheck } from 'lucide-react';

const features = [
  {
    icon: Server,
    title: 'Bring your own Postgres',
    body: 'All state lives there. No SQLite, no local disk, no time-series DB.',
  },
  {
    icon: Boxes,
    title: 'Sharded checks',
    body: 'Workers split endpoints between them. Kill one and its checks move within ~15s.',
  },
  {
    icon: LayoutDashboard,
    title: 'Public and private dashboards',
    body: 'From one deployment, on a path or their own hostname, behind OIDC or an admin login.',
  },
  {
    icon: Activity,
    title: 'Live updates',
    body: 'Results stream to the browser from any replica. Two years of history from rollups.',
  },
  {
    icon: Palette,
    title: 'Custom CSS',
    body: 'A documented set of CSS variables and class names to restyle every page.',
  },
  {
    icon: ShieldCheck,
    title: 'Helm chart',
    body: 'Split or combined mode, Gateway API routing, PDBs, network policies, metrics.',
  },
];

export default function HomePage() {
  return (
    <main className="flex flex-1 flex-col items-center px-4 py-16 md:py-24">
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img src="/seer-logo.svg" alt="" width={96} height={96} className="mb-6 dark:hidden" />
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img src="/seer-logo-white.svg" alt="" width={96} height={96} className="mb-6 hidden dark:block" />
      <h1 className="mb-4 text-center text-4xl font-bold tracking-tight md:text-5xl">Seer</h1>
      <p className="mb-8 max-w-xl text-center text-lg text-fd-muted-foreground">
        Highly available, self-hosted status pages and uptime monitoring. Configured from one
        YAML file; every replica is stateless.
      </p>
      <div className="mb-16 flex flex-wrap justify-center gap-3">
        <Link
          href="/docs/getting-started"
          className="rounded-lg bg-fd-primary px-5 py-2.5 font-medium text-fd-primary-foreground"
        >
          Get started
        </Link>
        <Link
          href="/docs/configuration"
          className="rounded-lg border px-5 py-2.5 font-medium hover:bg-fd-accent"
        >
          Configuration
        </Link>
      </div>
      <div className="grid w-full max-w-5xl gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {features.map(({ icon: Icon, title, body }) => (
          <div key={title} className="rounded-xl border bg-fd-card p-5">
            <Icon className="mb-3 size-5 text-fd-primary" />
            <h2 className="mb-1 font-semibold">{title}</h2>
            <p className="text-sm text-fd-muted-foreground">{body}</p>
          </div>
        ))}
      </div>
    </main>
  );
}
