const globalModel = {
  namespace: 'globalData',
  state: {
    currentEnv: '',
  },
  effects: {
    *currentEnv({ payload }, { put }) {
      yield put({
        type: 'setCurrentEnv', // 这就是reducer的addNum方法，put用来触发reducer中的方法，payload是传过去的参数。同时也能触发同等级effects中的方法
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
