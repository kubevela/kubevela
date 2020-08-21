import { getTraitByName, getTraits } from '@/services/trait.js';

const TestModel = {
  namespace: 'trait',
  state: {
    // initailState: '8880'
  },
  effects: {
    *getTraitByName({ payload }, { call }) {
      const res = yield call(getTraitByName, payload);
      return res;
    },
    *getTraits({ payload }, { call }) {
      const res = yield call(getTraits, payload);
      return res;
    },
  },
  reducers: {},
};
export default TestModel;
