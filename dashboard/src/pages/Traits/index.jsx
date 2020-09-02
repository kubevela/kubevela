import React, { Fragment } from 'react';
import { Spin } from 'antd';
import { connect } from 'dva';
import _ from 'lodash';
import Trait from '../../components/Trait';

@connect(({ loading }) => ({
  loadingAll: loading.models.trait,
}))
class TableList extends React.PureComponent {
  constructor(props) {
    super(props);
    this.state = {
      traitType: '',
      propsObj: {},
    };
  }

  componentDidMount() {
    const traitType = _.get(this.props, 'match.params.traitType', '');
    if (traitType) {
      this.getInitialData(traitType);
    }
  }

  // 组件更新时被调用
  UNSAFE_componentWillReceiveProps(nextProps) {
    const traitType = _.get(nextProps, 'match.params.traitType', '');
    const prevTraitType = this.state.traitType;
    // 这里不能直接调用会造成页面死循环，要判断前一个值是否相同
    if (traitType && traitType !== prevTraitType) {
      this.getInitialData(traitType);
      this.setState({
        traitType,
      });
    }
  }

  getInitialData = async (traitType) => {
    // eslint-disable-next-line no-param-reassign
    traitType = traitType.toLowerCase();
    const res = await this.props.dispatch({
      type: 'trait/getTraitByName',
      payload: {
        traitName: traitType,
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
