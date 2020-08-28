import React, { Fragment } from 'react';
import { Spin } from 'antd';
import { connect } from 'dva';
import Trait from '../../../components/Trait';

@connect(({ loading }) => ({
  loadingAll: loading.models.trait,
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
      type: 'trait/getTraitByName',
      payload: {
        traitName: 'rollout',
      },
    });
    if (res) {
      let propsObj = {};
      propsObj = {
        title: res.name,
        settings: res.parameters,
        crdInfo: res.crdInfo,
        appliesTo: res.appliesTo,
        btnValue: 'Attach to',
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
        {propsObj.title ? <Trait propsObj={propsObj} /> : <Fragment />}
      </Spin>
    );
  }
}

export default TableList;
