// @ts-check

/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docs: [
    'welcome',
    {
      type: 'category',
      label: 'Design',
      collapsed: false,
      items: ['architecture', 'domain-model', 'api', 'security'],
    },
    {
      type: 'category',
      label: 'Operations',
      collapsed: false,
      items: ['local-deployment', 'cloud-deployment', 'backup-restore'],
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
