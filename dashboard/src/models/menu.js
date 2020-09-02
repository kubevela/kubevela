import { capabilityList } from '@/services/capability.js';

function getMenuList(response) {
  let workloadList = [];
  let traitList = [];
  // eslint-disable-next-line no-param-reassign
  response = response.filter((item) => {
    return item.status === 'installed';
  });
  response.forEach((item) => {
    if (item.type === 'workload') {
      workloadList.push(item.name);
    } else if (item.type === 'trait') {
      traitList.push(item.name);
    }
  });
  // 在此之前要对workloadList和traitList进行一次去重操作
  workloadList = workloadList.map((item) => {
    // eslint-disable-next-line no-param-reassign
    item = item.charAt(0).toUpperCase() + item.slice(1);
    return {
      name: item,
      path: `/Workload/${item}`,
    };
  });
  traitList = traitList.map((item) => {
    // eslint-disable-next-line no-param-reassign
    item = item.charAt(0).toUpperCase() + item.slice(1);
    return {
      name: item,
      path: `/Traits/${item}`,
    };
  });
  // 只是动态生成侧边栏(name,path,icon)，路由还是config.js里面配置的路由
  const menuList = [
    {
      path: '/',
      redirect: `/ApplicationList`,
    },
    {
      name: 'ApplicationList',
      icon: 'Table',
      path: `/ApplicationList`,
    },
    {
      name: 'ApplicationList.ApplicationListDetail',
      hideInMenu: true,
      path: '/ApplicationList/ApplicationListDetail',
    },
    {
      name: 'ApplicationList.CreateApplication',
      hideInMenu: true,
      path: '/ApplicationList/CreateApplication',
    },
    {
      name: 'Workload',
      path: '/Workload',
      routes: [
        ...workloadList,
        {
          name: 'Detail',
          path: '/Workload/Detail',
          hideInMenu: true,
        },
      ],
    },
    {
      path: '/Traits',
      name: 'Traits',
      routes: [
        ...traitList,
        {
          name: 'Detail',
          path: '/Traits/Detail',
          hideInMenu: true,
        },
      ],
    },
    // {
    //   name: 'Release',
    //   path: '/Release',
    // },
    {
      name: 'Capability',
      path: '/Capability',
    },
    {
      path: '/System',
      name: 'System',
      routes: [
        {
          name: 'Env',
          path: '/System/Env',
        },
      ],
    },
    {
      name: 'Capability.Detail',
      hideInMenu: true,
      path: '/Capability/Detail',
    },
  ];
  return menuList;
}

const TestModel = {
  namespace: 'menus',
  state: {
    menuData: [],
  },
  effects: {
    *getMenuData({ payload }, { call, put }) {
      let response = yield call(capabilityList, payload);
      response = getMenuList(response);
      yield put({
        type: 'saveMenuData',
        payload: response,
      });
    },
  },
  reducers: {
    saveMenuData(state, action) {
      return {
        ...state,
        menuData: action.payload || [],
      };
    },
  },
};
export default TestModel;
