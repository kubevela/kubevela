import { getTraitByName, getTraits, attachOneTraits, deleteOneTrait } from '@/services/trait.js';

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
    *attachOneTraits({ payload }, { call }) {
      const res = yield call(attachOneTraits, payload);
      return res;
    },
    *deleteOneTrait({ payload }, { call }) {
      const res = yield call(deleteOneTrait, payload);
      return res;
    },
  },
  reducers: {},
};
export default TestModel;
