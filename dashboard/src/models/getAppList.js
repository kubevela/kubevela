import { doit } from "@/services/getAppList";

const TestModel = {
  namespace: 'testapi',
  state: {
    // initailState: '8880'
  },
  effects: {
    *getList({ payload, callback }, { call, put }) {
      const res = yield call(doit, payload)
      //doit是引入services层那个js文件的doit方法，payload是后台要求传递的参数，res就是后台返过来的数据
      yield put({
        type: 'addList', //这就是reducer的addNum方法，put用来触发reducer中的方法，payload是传过去的参数。同时也能触发同等级effects中的方法
        payload: {
          returnObj: res//把后台返回的数据赋值给num,假如哪个reducer中的方法是由这里effects去触发的，哪个num名必须是这里的名字num，如果reducer中的方法不是这触发，那名字可以随意取
        }
      })
    }
  },
  reducers: {
    addList(state, { payload: { returnObj } }) {
      return { ...state, returnObj }
    }
  }
};
export default TestModel;