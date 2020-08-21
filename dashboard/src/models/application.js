import { getapplist, createApp } from '@/services/application';

const TestModel = {
  namespace: 'applist',
  state: {
    // initailState: '8880'
  },
  effects: {
    *getList({ payload }, { call, put }) {
      const res = yield call(getapplist, payload);
      // getlist是引入services层那个js文件的getlist方法，payload是后台要求传递的参数，res就是后台返过来的数据
      yield put({
        type: 'addList', // 这就是reducer的addNum方法，put用来触发reducer中的方法，payload是传过去的参数。同时也能触发同等级effects中的方法
        payload: {
          returnObj: res, // 把后台返回的数据赋值给num,假如哪个reducer中的方法是由这里effects去触发的，哪个num名必须是这里的名字num，如果reducer中的方法不是这触发，那名字可以随意取
        },
      });
    },
    *createApp({ payload }, { call }) {
      // 如果 method = Get，data 类型 =  list/json 否则，data 类型 =  string，存储的是操作成功的信息
      // 非get请求，将结果返回，在调用页面进行async await 来进行操作结果提示
      const res = yield call(createApp, payload);
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
