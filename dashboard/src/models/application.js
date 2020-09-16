import { getapplist, createApp, getAppDetail, deleteApp } from '@/services/application';

const TestModel = {
  namespace: 'applist',
  state: {},
  effects: {
    *getList({ payload }, { call, put }) {
      const res = yield call(getapplist, payload);
      // getlist是引入services层那个js文件的getlist方法，payload是后台要求传递的参数，res就是后台返过来的数据
      yield put({
        type: 'addList',
        payload: {
          returnObj: res,
        },
      });
    },
    *createApp({ payload }, { call }) {
      // 如果 method = Get，data 类型 =  list/json 否则，data 类型 =  string，存储的是操作成功的信息
      // 非get请求，将结果返回，在调用页面进行async await 来进行操作结果提示
      const res = yield call(createApp, payload);
      return res;
    },
    *getAppDetail({ payload }, { call }) {
      const res = yield call(getAppDetail, payload);
      return res;
    },
    *deleteApp({ payload }, { call }) {
      const res = yield call(deleteApp, payload);
      return res;
    },
  },
  reducers: {
    addList(state, { payload: { returnObj } }) {
      return { ...state, returnObj };
    },
  },
};
export default TestModel;
