import React from "react";
import Workload from '../../../components/Workload'

class TableList extends React.Component {
  render() {
    let propsObj = {
      title: 'Deployment',
      settings: [{
        name: 'Deployment Strategy',
        value: 'RollingUpdate'
      },{
        name: 'Rolling Update Strategy',
        value: 'Max Surge 25%, Max Unavaiable 25%'
      },{
        name: 'Min Ready Seconds',
        value: 0
      },{
        name: 'Revision History Limit',
        value: 10
      },{
        name: 'Replicas',
        value: 0
      }],
      pathname:'/ApplicationList/CreateApplication',
      state:{
        WorkloadType:'Deployment'
      },
      btnValue: 'Create',
      hrefAddress: '#',
      btnIsShow: true
    }
    return (
      <Workload propsObj={propsObj}/>
    );
  }
}

export default TableList;