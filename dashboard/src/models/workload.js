import { createWorkload, getWorkload, getWorkloadByName } from '@/services/workload.js';

const TestModel = {
  namespace: 'workload',
  state: {
    // initailState: '8880'
  },
  effects: {
    *createWorkload({ payload }, { call }) {
      const res = yield call(createWorkload, payload);
      return res;
    },
    *getWorkload({ payload }, { call }) {
      const res = yield call(getWorkload, payload);
      return res;
    },
    *getWorkloadByName({ payload }, { call }) {
      const res = yield call(getWorkloadByName, payload);
      return res;
    },
  },
  reducers: {},
};
export default TestModel;
