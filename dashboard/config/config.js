// https://umijs.org/config/
import {defineConfig} from 'umi';
import defaultSettings from './defaultSettings';
import proxy from './proxy';

const {REACT_APP_ENV} = process.env;
export default defineConfig({
  hash: true,
  antd: {},
  dva: {
    hmr: true,
  },
  locale: {
    // default zh-CN
    default: 'en-US',
    // default true, when it is true, will use `navigator.language` overwrite default
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
              // redirect: `/${envname}/ApplicationList`,
              redirect: `/ApplicationList`,
            },
            {
              name: 'ApplicationList',
              icon: 'table',
              // path: `/${envname}/ApplicationList`,
              path: `/ApplicationList`,
              component: './ApplicationList',
            },
            {
              name: 'ApplicationList.ApplicationListDetail',
              hideInMenu: true,
              path: '/ApplicationList/ApplicationListDetail',
              component: './ApplicationList/ApplicationListDetail',
            },
            {
              name: 'ApplicationList.CreateApplication',
              hideInMenu: true,
              path: '/ApplicationList/CreateApplication',
              component: './ApplicationList/CreateApplication',
            },
            {
              name: 'Workload',
              icon: 'table',
              path: '/Workload',
              routes: [
                {
                  name: 'Deployment',
                  icon: 'table',
                  path: '/Workload/Deployment',
                  component: './Workload/Deployment',
                },
                {
                  name: 'Containerized',
                  icon: 'smile',
                  path: '/Workload/Containerized',
                  component: './Workload/Containerized',
                },
                {
                  name: 'Detail',
                  icon: 'smile',
                  path: '/Workload/Detail',
                  component: './Workload/Detail',
                  hideInMenu: true,
                },
              ],
            },
            {
              path: '/Traits',
              name: 'Traits',
              icon: 'table',
              routes: [
                {
                  name: 'Autoscaling',
                  icon: 'table',
                  path: '/Traits/Autoscaling',
                  component: './Traits/Autoscaling',
                },
                {
                  name: 'Rollout',
                  icon: 'smile',
                  path: '/Traits/Rollout',
                  component: './Traits/Rollout',
                },
                {
                  name: 'Detail',
                  icon: 'smile',
                  path: '/Traits/Detail',
                  component: './Traits/Detail',
                  hideInMenu: true,
                },
              ],
            },
            {
              name: 'Release',
              icon: 'table',
              path: '/Release',
              component: './Release',
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
                }
              ]
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
    // ...darkTheme,
    'primary-color': defaultSettings.primaryColor,
  },
  // @ts-ignore
  title: false,
  ignoreMomentLocale: true,
  proxy: proxy[REACT_APP_ENV || 'dev'],
  manifest: {
    basePath: '/',
  },
});
