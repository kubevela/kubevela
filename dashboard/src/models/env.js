import { getEnvs, switchEnv, initialEnvs, deleteEnv } from '@/services/env';

const TestModel = {
  namespace: 'envs',
  state: {
    envs: undefined,
  },
  effects: {
    *getEnvs({ payload }, { call, put }) {
      const res = yield call(getEnvs, payload);
      yield put({
        type: 'onGetEnvsSuccess',
        payload: res,
      });
      return res;
    },
    *switchEnv({ payload }, { call }) {
      const res = yield call(switchEnv, payload);
      return res;
    },
    *initialEnvs({ payload }, { call, put }) {
      yield call(initialEnvs, payload);
      const res = yield call(getEnvs, payload);
      yield put({
        type: 'onGetEnvsSuccess',
        payload: res,
      });
      return res;
    },
    *deleteEnv({ payload }, { call, put }) {
      yield call(deleteEnv, payload);
      const res = yield call(getEnvs);
      yield put({
        type: 'onGetEnvsSuccess',
        payload: res,
      });
      return res;
    },
  },
  reducers: {
    onGetEnvsSuccess(state, { payload }) {
      return {
        ...state,
        envs: payload,
      };
    },
  },
};
export default TestModel;
