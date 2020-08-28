import {
  getCapabilityCenterlist,
  createCapabilityCenter,
  syncCapability,
  deleteCapability,
  syncOneCapability,
  deleteOneCapability,
  capabilityList,
} from '@/services/capability.js';

const TestModel = {
  namespace: 'capability',
  state: {
    // initailState: '8880'
  },
  effects: {
    *getCapabilityCenterlist({ payload }, { call }) {
      const res = yield call(getCapabilityCenterlist, payload);
      return res;
    },
    *createCapabilityCenter({ payload }, { call }) {
      // 如果 method = Get，data 类型 =  list/json 否则，data 类型 =  string，存储的是操作成功的信息
      // 非get请求，将结果返回，在调用页面进行async await 来进行操作结果提示
      const res = yield call(createCapabilityCenter, payload);
      return res;
    },
    *syncCapability({ payload }, { call }) {
      const res = yield call(syncCapability, payload);
      return res;
    },
    *deleteCapability({ payload }, { call }) {
      const res = yield call(deleteCapability, payload);
      return res;
    },
    *syncOneCapability({ payload }, { call }) {
      const res = yield call(syncOneCapability, payload);
      return res;
    },
    *deleteOneCapability({ payload }, { call }) {
      const res = yield call(deleteOneCapability, payload);
      return res;
    },
    *capabilityList({ payload }, { call }) {
      const res = yield call(capabilityList, payload);
      return res;
    },
  },
  reducers: {},
};
export default TestModel;
