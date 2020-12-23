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
        name: 'operating',
        path: '/capabilities/operating',
        component: './Capability/Operating',
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
