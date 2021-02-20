export default [
  {
    path: '/',
    redirect: `/applications`,
  },
  {
    name: 'applications',
    icon: 'appstore',
    path: `/applications`,
    routes: [
      {
        path: '/applications',
        component: './Application',
      },
      {
        name: 'create',
        path: '/applications/create',
        component: './Application/Create',
        hideInMenu: true,
      },
    ],
  },
  {
    name: 'capability',
    icon: 'AppstoreAddOutlined',
    path: '/capabilities',
    routes: [
      {
        path: '/capabilities',
        redirect: `/Capability/Workloads`,
      },
      {
        name: 'workloads',
        path: '/capabilities/workloads',
        component: './Capability/Workloads',
      },
      {
        name: 'traits',
        path: '/capabilities/traits',
        component: './Capability/Traits',
      },
    ],
  },
  {
    name: 'system',
    icon: 'setting',
    path: '/system',
    routes: [
      {
        path: '/system',
        redirect: `/system/environment`,
      },
      {
        name: 'environment',
        path: '/system/environment',
        component: './System/Environment',
      },
    ],
  },
  {
    component: './404',
  },
];
