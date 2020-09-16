const globalModel = {
  namespace: 'globalData',
  state: {
    currentEnv: '',
  },
  effects: {
    *currentEnv({ payload }, { put }) {
      yield put({
        type: 'setCurrentEnv',
        payload,
      });
    },
  },
  reducers: {
    setCurrentEnv(state, { payload: { currentEnv } }) {
      return { ...state, currentEnv };
    },
  },
};
export default globalModel;
