import { getComponentList, getComponentDetail, deleteComponent } from '@/services/components.js';

const TestModel = {
  namespace: 'components',
  state: {},
  effects: {
    *getComponentList({ payload }, { call }) {
      const res = yield call(getComponentList, payload);
      return res;
    },
    *getComponentDetail({ payload }, { call }) {
      const res = yield call(getComponentDetail, payload);
      return res;
    },
    *deleteComponent({ payload }, { call }) {
      const res = yield call(deleteComponent, payload);
      return res;
    },
  },
  reducers: {},
};
export default TestModel;
