// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docs: [
    'welcome',
    {
      type: 'category',
      label: 'Design',
      collapsed: false,
      items: ['architecture', 'domain-model', 'api', 'security', 'payments'],
    },
    {
      type: 'category',
      label: 'Operations',
      collapsed: false,
      items: ['setup', 'local-deployment', 'cloud-deployment', 'backup-restore'],
    },
    {
      type: 'category',
      label: 'Decision records',
      link: { type: 'doc', id: 'adr/index' },
      items: [
        'adr/0001-persistence-stack',
        'adr/0002-single-binary-cli',
        'adr/0003-docs-site',
        'adr/0004-tenant-context-and-roles',
        'adr/0005-first-run-setup',
        'adr/0006-cli-scope',
        'adr/0007-online-payments',
      ],
    },
    {
      type: 'category',
      label: 'Community',
      link: { type: 'doc', id: 'community/contribution' },
      items: ['community/contribution', 'community/security-reporting'],
    },
  ],
};

export default sidebars;
