import React, { Fragment } from 'react';
import { connect } from 'dva';
import { Spin } from 'antd';
import Workload from '../../../components/Workload';

@connect(({ loading }) => ({
  loadingAll: loading.models.workload,
}))
class TableList extends React.PureComponent {
  constructor(props) {
    super(props);
    this.state = {
      propsObj: {},
    };
  }

  componentDidMount() {
    this.getInitialData();
  }

  getInitialData = async () => {
    const res = await this.props.dispatch({
      type: 'workload/getWorkloadByName',
      payload: {
        workloadName: 'deployment',
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
