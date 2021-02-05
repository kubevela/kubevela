export default [
  {
    path: '/',
    redirect: `/System`,
  },
  {
    name: 'applications',
    icon: 'appstore',
    path: `/applications`,
    component: './Application',
  },
  /* Application Create should be moved to /Application */
  {
    name: 'create_application',
    path: '/applications/create',
    component: './CreateApplication'
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
        component: './Capability/Workloads'
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
    path: '/System',
    routes: [
      {
        path: '/System',
        redirect: `/System/Environment`,
      },
      {
        name: 'environment',
        path: '/System/Environment',
        component: './System/Environment',
      },
    ],
  },
  {
    component: './404',
  },
];
