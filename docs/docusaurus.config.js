// @ts-check
// See: https://docusaurus.io/docs/api/docusaurus-config

import { themes as prismThemes } from 'prism-react-renderer';

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'servly',
  tagline: 'Lightweight order management for restaurants.',
  favicon: 'img/icon.svg',

  // v4 turns on the Rspack-based "Faster" build, hence @docusaurus/faster in package.json.
  future: {
    v4: true,
    faster: true,
  },

  // GitHub Pages: https://servly-io.github.io/servly/
  url: 'https://servly-io.github.io',
  // GitHub Pages serves under /servly/; `make docs-serve` sets DOCS_BASE_URL=/
  // so the local dev server answers at http://localhost:<port>/.
  baseUrl: process.env.DOCS_BASE_URL || '/servly/',
  organizationName: 'servly',
  projectName: 'servly',

  onBrokenLinks: 'throw',
  onBrokenAnchors: 'throw',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          sidebarPath: require.resolve('./sidebars.js'),
          // The docs are the site: no landing page in front of them.
          routeBasePath: '/',
          editUrl: 'https://github.com/servly-io/servly/edit/main/docs/',
          // docs/ tracks main. A release cuts a snapshot with
          // `npm run docusaurus docs:version <x.y>`, which lands in
          // versioned_docs/ and shows up in the version dropdown.
          lastVersion: 'current',
          versions: {
            current: { label: 'main' },
          },
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  themes: [
    '@docusaurus/theme-mermaid',
    [
      require.resolve('@easyops-cn/docusaurus-search-local'),
      /** @type {import("@easyops-cn/docusaurus-search-local").PluginOptions} */
      ({
        hashed: true,
        docsDir: 'docs',
        language: ['en'],
        indexDocs: true,
        indexBlog: false,
        indexPages: false,
        docsRouteBasePath: '/',
        searchResultLimits: 10,
        searchBarShortcut: true,
        removeDefaultStemmer: true,
        searchBarShortcutHint: true,
        highlightSearchTermsOnTargetPage: true,
      }),
    ],
  ],

  markdown: {
    mermaid: true,
    hooks: {
      onBrokenMarkdownLinks: 'throw',
    },
  },

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      image: 'img/logo-horizontal.svg',
      colorMode: {
        defaultMode: 'dark',
        disableSwitch: false,
        respectPrefersColorScheme: false,
      },
      docs: {
        sidebar: {
          hideable: true,
        },
      },
      navbar: {
        title: 'servly',
        logo: {
          alt: 'servly',
          src: 'img/logo-horizontal.svg',
          srcDark: 'img/logo-horizontal-dark.svg',
        },
        items: [
          {
            type: 'docSidebar',
            sidebarId: 'docs',
            position: 'left',
            label: 'Docs',
          },
          {
            type: 'docsVersionDropdown',
            position: 'right',
            dropdownActiveClassDisabled: true,
          },
          {
            href: 'https://github.com/servly-io/servly',
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Project',
            items: [
              { label: 'Source code', href: 'https://github.com/servly-io/servly' },
              { label: 'Releases', href: 'https://github.com/servly-io/servly/releases' },
              { label: 'Issues', href: 'https://github.com/servly-io/servly/issues' },
            ],
          },
          {
            title: 'Docs',
            items: [
              { label: 'Architecture', to: 'architecture' },
              { label: 'API', to: 'api' },
              { label: 'Local deployment', to: 'local-deployment' },
            ],
          },
          {
            title: 'Community',
            items: [
              { label: 'Contributing', to: 'community/contribution' },
              { label: 'Security', to: 'security' },
            ],
          },
        ],
        copyright: `© ${new Date().getFullYear()} servly authors. MIT License.`,
      },
      prism: {
        theme: prismThemes.github,
        darkTheme: prismThemes.dracula,
        additionalLanguages: ['bash', 'ini', 'diff', 'sql', 'json', 'yaml'],
      },
    }),
};

export default config;
