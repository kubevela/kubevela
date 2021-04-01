/** @type {import('@docusaurus/types').DocusaurusConfig} */
module.exports = {
  title: 'KubeVela',
  tagline: 'Make shipping applications more enjoyable.',
  url: 'https://kubevela.io',
  baseUrl: '/',
  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'warn',
  favicon: 'img/favicon.ico',
  organizationName: 'oam-dev', // Usually your GitHub org/user name.
  projectName: 'kubevela.io', // Usually your repo name.
  i18n: {
    defaultLocale: 'en',
    locales: ['en', 'zh'],
    localeConfigs: {
      en: {
        label: 'English',
      },
      zh: {
        label: '简体中文',
      },
    },
  },
  themeConfig: {
    navbar: {
      title: 'KubeVela',
      logo: {
        alt: 'KubeVela',
        src: 'img/logo.svg',
        srcDark: 'img/logoDark.svg',
      },
      items: [
        {
          type: 'docsVersionDropdown',
          position: 'right',
        },
        {
          to: 'docs/',
          activeBasePath: 'docs',
          label: 'Documentation',
          position: 'left',
        },
        {
          to: 'blog',
          label: 'Blog',
          position: 'left'
        },
        {
          type: 'localeDropdown',
          position: 'right',
        },
        {
          href: 'https://github.com/oam-dev/kubevela',
          className: 'header-githab-link',
          position: 'right',
        },
      ],
    },
    footer: {
      links: [
        {
          title: 'Documentation',
          items: [
            {
              label: 'Getting Started',
              to: '/docs/install',
            },
            {
              label: 'Platform Builder Guide',
              to: '/docs/platform-engineers/overview',
            },
            {
              label: 'Developer Experience Guide',
              to: '/docs/quick-start-appfile',
            },
          ],
        },
        {
          title: 'Community',
          items: [
            {
              label: 'Slack ( #kubevela channel )',
              href: 'https://slack.cncf.io/'
            },
            {
              label: 'Gitter',
              href: 'https://gitter.im/oam-dev/community',
            },
            {
              label: 'DingTalk (23310022)',
              href: '.',
            }
          ],
        },
        {
          title: 'More',
          items: [
            {
              label: 'GitHub',
              href: 'https://github.com/oam-dev/kubevela',
            },
            {
              label: 'Blog',
              to: 'blog',
            },
          ],
        },
      ],
      copyright: `
        <br />
        <strong>© KubeVela Authors ${new Date().getFullYear()} | Documentation Distributed under <a herf="https://creativecommons.org/licenses/by/4.0">CC-BY-4.0</a> </strong>
        <br />
      `,
    },
    prism: {
      theme: require('prism-react-renderer/themes/dracula'),
    },
  },
  plugins: [
    [
      require.resolve("@easyops-cn/docusaurus-search-local"),
      {
        hashed: true,
        language: ["en", "zh"],
        indexBlog: true,
      },
    ],
  ],
  presets: [
    [
      '@docusaurus/preset-classic',
      {
        docs: {
          sidebarPath: require.resolve('./sidebars.js'),
          editUrl: function ({
            locale,
            docPath,
          }) {
            return `https://github.com/oam-dev/kubevela/edit/master/docs/${locale}/${docPath}`;
          },
          showLastUpdateAuthor: true,
          showLastUpdateTime: true,
          includeCurrentVersion: true,
          lastVersion: 'current',
          // versions: {
          //   current: {
          //     label: 'master',
          //     path: '/',
          //   },
          // },
        },
        blog: {
          showReadingTime: true,
          editUrl:
            'https://github.com/oam-dev/kubevela.io/tree/main/blog',
        },
        theme: {
          customCss: require.resolve('./src/css/custom.css'),
        },
      },
    ],
  ],
};
