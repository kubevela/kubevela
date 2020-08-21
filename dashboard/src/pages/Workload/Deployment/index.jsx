import React from 'react';
import { connect } from 'dva';
import Workload from '../../../components/Workload';

@connect(({ loading }) => ({
  loadingAll: loading.models.applist,
}))
class TableList extends React.PureComponent {
  // async componentDidMount(){
  //   await this.props.dispatch({
  //     type:'workload/getWorkloadByName',
  //     payload: {
  //       workloadName: 'containerized'
  //     }
  //   })
  // };

  render() {
    const propsObj = {
      title: 'Deployment',
      settings: [
        {
          name: 'Deployment Strategy',
          value: 'RollingUpdate',
        },
        {
          name: 'Rolling Update Strategy',
          value: 'Max Surge 25%, Max Unavaiable 25%',
        },
        {
          name: 'Min Ready Seconds',
          value: 0,
        },
        {
          name: 'Revision History Limit',
          value: 10,
        },
        {
          name: 'Replicas',
          value: 0,
        },
      ],
      pathname: '/ApplicationList/CreateApplication',
      state: {
        WorkloadType: 'Deployment',
      },
      btnValue: 'Create',
      hrefAddress: '#',
      btnIsShow: true,
    };
    return <Workload propsObj={propsObj} />;
  }
}

export default TableList;
