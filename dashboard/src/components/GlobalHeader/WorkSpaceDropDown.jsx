import { Menu, Dropdown, message } from 'antd';
import { DownOutlined } from '@ant-design/icons';
import React from 'react';
import { connect } from 'dva';
import './WorkSpaceDropDown.css'

@connect((env) => ({ envs: env.envs }))
export default class WorkSpaceDropDown extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      workSpaceName: '',
      envs: [],
      namespace: ''
    };
  }

  async componentDidMount() {
    const envs = await this.props.dispatch({
      type: 'envs/getEnvs',
    });
    if (envs) {
      const { name, namespace } = envs.find((env) => {
        return env.current === '*';
      });
      this.setState({
        envs,
        workSpaceName: name,
        namespace: namespace
      });
      this.props.dispatch({
        type: 'globalData/currentEnv',
        payload: {
          currentEnv: name,
        },
      });
    }
  }

  handleMenuClick = async (e) => {
    // 发送切换envs的接口
    const switchResult = await this.props.dispatch({
      type: 'envs/switchEnv',
      payload: {
        currentEnv: e.key,
      },
    });
    if (switchResult) {
      message.success(switchResult);
    }
    this.setState(
      {
        workSpaceName: e.key,
        namespace: e.item.props.title
      },
      () => {
        // 值切换存储
        this.props.dispatch({
          type: 'globalData/currentEnv',
          payload: {
            currentEnv: e.key,
          },
        });
      },
    );
    await this.props.dispatch({
      type: 'envs/getEnvs', // applist对应models层的命名空间namespace
    });
  };

  render() {
    const { envs } = this.props;
    const menu = (
      <Menu onClick={ this.handleMenuClick }>
        { envs.envs && envs.envs.map((item) => {
          return <Menu.Item key={ item.name } title={ item.namespace }>
            <div className='box'>
              <div className="box1">{ item.name }</div>
              <div className="box2">{ item.namespace }</div>
            </div>
          </Menu.Item>;
        }) }
      </Menu>
    );
    return (
      <Dropdown overlay={ menu }>
        <div className='drop-box'>
          <div className='btn-box'>
            <div className="btn-top">{ this.state.workSpaceName }</div>
            <div className="btn-bottom">{ this.state.namespace }</div>
          </div>
          <DownOutlined style={ { fontSize: '15px', color: '#ffffff' } }/>
        </div>
      </Dropdown>
    );
  }
}
