// https://umijs.org/config/
import { defineConfig } from 'umi';
import defaultSettings from './defaultSettings';
import proxy from './proxy';

const { REACT_APP_ENV } = process.env;
export default defineConfig({
  history: { type: 'hash' },
  hash: false,
  antd: {},
  dva: {
    hmr: true,
  },
  locale: {
    default: 'en-US',
    antd: false,
    baseNavigator: false,
  },
  dynamicImport: {
    loading: '@/components/PageLoading/index',
  },
  targets: {
    ie: 11,
  },
  // umi routes: https://umijs.org/docs/routing
  routes: [
    {
      path: '/',
      component: '../layouts/SecurityLayout',
      routes: [
        {
          path: '/',
          component: '../layouts/BasicLayout',
          routes: [
            {
              path: '/',
              redirect: `/ApplicationList`,
            },
            {
              name: 'ApplicationList',
              icon: 'table',
              path: `/ApplicationList`,
              component: './ApplicationList',
            },
            {
              name: 'ApplicationList.WorkloadDetail',
              icon: 'smile',
              path: '/ApplicationList/WorkloadDetail',
              component: './Workload/Detail',
              hideInMenu: true,
            },
            {
              name: 'ApplicationList.TraitDetail',
              icon: 'smile',
              path: '/ApplicationList/TraitDetail',
              component: './Traits/Detail',
              hideInMenu: true,
            },
            {
              name: 'ApplicationList.Components',
              hideInMenu: true,
              path: '/ApplicationList/:appName/Components',
              component: './ApplicationList/Components',
            },
            {
              name: 'ApplicationList.Components.createComponent',
              hideInMenu: true,
              path: '/ApplicationList/:appName/createComponent',
              component: './ApplicationList/CreateComponent',
            },
            {
              name: 'Workload',
              icon: 'table',
              path: '/Workload',
              routes: [
                {
                  name: 'WorkloadItem',
                  icon: 'smile',
                  path: '/Workload/:WorkloadType',
                  component: './Workload/index.jsx',
                },
              ],
            },
            {
              path: '/Traits',
              name: 'Traits',
              icon: 'table',
              routes: [
                {
                  name: 'TraitItem',
                  icon: 'smile',
                  path: '/Traits/:traitType',
                  component: './Traits/index.jsx',
                },
              ],
            },
            {
              name: 'Capability',
              icon: 'table',
              path: '/Capability',
              component: './Capability',
            },
            {
              path: '/System',
              name: 'System',
              icon: 'table',
              routes: [
                {
                  name: 'Env',
                  icon: 'table',
                  path: '/System/Env',
                  component: './System/Env',
                },
              ],
            },
            {
              name: 'Capability.Detail',
              hideInMenu: true,
              path: '/Capability/Detail',
              component: './Capability/Detail',
            },
            {
              component: './404',
            },
          ],
        },
        {
          component: './404',
        },
      ],
    },
    {
      component: './404',
    },
  ],
  // Theme for antd: https://ant.design/docs/react/customize-theme-cn
  theme: {
    // 主题配置
    'primary-color': defaultSettings.primaryColor,
    'link-color': defaultSettings.linkColor,
    'link-hover-color': defaultSettings.linkHoverColor,
    'disabled-bg': defaultSettings.disabledBg,
    'disabled-color': defaultSettings.disabledColor,
    'btn-disable-color': defaultSettings.btnDisableColor,
  },
  // @ts-ignore
  title: false,
  ignoreMomentLocale: true,
  proxy: proxy[REACT_APP_ENV || 'dev'],
  manifest: {
    basePath: '/',
  },
});
