import {Menu, Dropdown, Button, message, Row, Col, Divider} from 'antd';
import {DownOutlined} from '@ant-design/icons';
import React from 'react';
import {connect} from 'dva';

@connect((env) => ({envs: env.envs}))
export default class WorkSpaceDropDown extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      workSpaceName: '',
      envs: [],
    };
  }

  async componentDidMount() {
    const envs = await this.props.dispatch({
      type: 'envs/getEnvs', // applist对应models层的命名空间namespace
    });
    if (envs) {
      const {name = 'test'} = envs.find((a) => {
        return a.current === '*';
      });
      this.setState({
        envs,
        workSpaceName: name,
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
    const {envs} = this.props;
    const menu = (
      <Menu onClick={this.handleMenuClick}>
        {/* <Menu.Item key="default">default</Menu.Item>
        <Menu.Item key="am-system">oam-system</Menu.Item>
        <Menu.Item key="linkerd">linkerd</Menu.Item>
        <Menu.Item key="rio-system">rio-system</Menu.Item> */}
        {envs.envs && envs.envs.map((item) => {
          return <Menu.Item key={item.name}>{item.name}</Menu.Item>;
        })}
      </Menu>
    );
    return (
      <Dropdown overlay={menu}>
        <Button style={{marginTop: '10px'}}>
          {this.state.workSpaceName} <DownOutlined/>
        </Button>
      </Dropdown>
    );
  }
}
