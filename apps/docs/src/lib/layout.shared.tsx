import type { BaseLayoutProps } from 'fumadocs-ui/layouts/shared';
import { appName, gitConfig } from './shared';

export function baseOptions(): BaseLayoutProps {
  return {
    nav: {
      title: (
        <>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="/seer-logo.svg" alt="" width={24} height={24} className="dark:hidden" />
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src="/seer-logo-white.svg" alt="" width={24} height={24} className="hidden dark:block" />
          <span className="font-semibold">{appName}</span>
        </>
      ),
    },
    githubUrl: `https://github.com/${gitConfig.user}/${gitConfig.repo}`,
  };
}
