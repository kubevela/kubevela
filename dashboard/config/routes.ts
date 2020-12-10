export default [
  {
    path: '/',
    redirect: `/System`,
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
