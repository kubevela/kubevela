import { getEnvs, switchEnv } from '@/services/env';

const TestModel = {
  namespace: 'envs',
  state: {
    // initailState: '8880'
  },
  effects: {
    *getEnvs({ payload }, { call }) {
      const res = yield call(getEnvs, payload);
      return res;
    },
    *switchEnv({ payload }, { call }) {
      const res = yield call(switchEnv, payload);
      return res;
    },
  },
  reducers: {},
};
export default TestModel;
