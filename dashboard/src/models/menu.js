import { getTraits } from '@/services/trait.js';
import { getWorkload } from '@/services/workload.js';

function getMenuList(workload, trait) {
  let workloadList = [];
  let traitList = [];
  if (workload) {
    workloadList = workload.map((item) => {
      let name1 = item.name;
      name1 = name1.charAt(0).toUpperCase() + name1.slice(1);
      return {
        name: name1,
        path: `/Workload/${name1}`,
        key: name1,
      };
    });
  }
  if (trait) {
    traitList = trait.map((item) => {
      let name1 = item.name;
      name1 = name1.charAt(0).toUpperCase() + name1.slice(1);
      return {
        name: name1,
        path: `/Traits/${name1}`,
        key: name1,
      };
    });
  }
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
      key: 'applist',
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
    {
      name: 'Capability',
      path: '/Capability',
      key: 'Capability',
    },
    {
      path: '/System',
      name: 'System',
      routes: [
        {
          name: 'Env',
          path: '/System/Env',
          key: 'Env',
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
      const workloadList = yield call(getWorkload, payload);
      const traitList = yield call(getTraits, payload);
      const response = getMenuList(workloadList, traitList);
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
