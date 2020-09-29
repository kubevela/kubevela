import React, { Fragment } from 'react';
import { connect } from 'dva';
import { Spin } from 'antd';
import _ from 'lodash';
import Workload from '../../components/Workload';

@connect(({ loading }) => ({
  loadingAll: loading.models.workload,
}))
class TableList extends React.PureComponent {
  constructor(props) {
    super(props);
    this.state = {
      propsObj: {},
      workloadType: '',
    };
  }

  componentDidMount() {
    const workloadType = _.get(this.props, 'match.params.WorkloadType', '');
    if (workloadType) {
      this.getInitialData(workloadType);
    }
  }

  // 组件更新时被调用
  UNSAFE_componentWillReceiveProps(nextProps) {
    const workloadType = _.get(nextProps, 'match.params.WorkloadType', '');
    const prevWorkloadType = this.state.workloadType;
    // 这里不能直接调用会造成页面死循环，要判断前一个值是否相同
    if (workloadType && workloadType !== prevWorkloadType) {
      this.getInitialData(workloadType);
      this.setState({
        workloadType,
      });
    }
  }

  getInitialData = async (workloadType) => {
    // eslint-disable-next-line no-param-reassign
    workloadType = workloadType.toLowerCase();
    const res = await this.props.dispatch({
      type: 'workload/getWorkloadByName',
      payload: {
        workloadName: workloadType,
      },
    });
    if (res) {
      let propsObj = {};
      propsObj = {
        title: res.name,
        settings: res.parameters,
        pathname: '/ApplicationList/CreateApplication',
        state: {
          WorkloadType: res.name,
        },
        crdInfo: res.crdInfo,
        btnValue: 'Create',
        hrefAddress: '#',
        btnIsShow: true,
        history: this.props.history,
      };
      this.setState({
        propsObj,
      });
    }
  };

  render() {
    let { loadingAll } = this.props;
    loadingAll = loadingAll || false;
    const { propsObj } = this.state;
    return (
      <Spin spinning={loadingAll}>
        {propsObj.title ? <Workload propsObj={propsObj} /> : <Fragment />}
      </Spin>
    );
  }
}

export default TableList;
