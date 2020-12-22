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
    path: '/Capability',
    routes: [
      {
        path: '/Capability',
        redirect: `/Capability/Workloads`,
      },
      {
        name: 'workloads',
        path: '/Capability/Workloads',
        component: './Capability/Workloads'
      },
      {
        name: 'operating',
        path: '/System/Environment',
        component: './System/Environment',
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
