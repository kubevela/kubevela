import React from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { BranchesOutlined, ApartmentOutlined } from '@ant-design/icons';
import { Button, Card, Row, Col, Form, Spin, Empty, Breadcrumb, Modal, Input } from 'antd';
import { connect } from 'dva';
import moment from 'moment';
import './index.less';
import { Link } from 'umi';

const layout = {
  labelCol: {
    span: 6,
  },
  wrapperCol: {
    span: 18,
  },
};

@connect(({ loading, applist, globalData }) => ({
  loadingAll: loading.models.applist,
  returnObj: applist.returnObj,
  currentEnv: globalData.currentEnv,
}))
class TableList extends React.Component {
  formRef = React.createRef();

  constructor(props) {
    super(props);
    this.state = {
      visible: false,
    };
  }

  componentDidMount() {
    const { currentEnv } = this.props;
    if (currentEnv) {
      this.props.dispatch({
        type: 'applist/getList',
        payload: {
          url: `/api/envs/${currentEnv}/apps/`,
        },
      });
    }
  }

  shouldComponentUpdate(nextProps) {
    if (nextProps.currentEnv === this.props.currentEnv) {
      return true;
    }
    this.props.dispatch({
      type: 'applist/getList',
      payload: {
        url: `/api/envs/${nextProps.currentEnv}/apps/`,
      },
    });
    return true;
  }

  showModal = () => {
    this.setState({
      visible: true,
    });
  };

  handleOk = async () => {
    await this.formRef.current.validateFields();
    // const submitData = await this.formRef.current.validateFields();
  };

  handleCancel = () => {
    this.setState({
      visible: false,
    });
  };

  onFinish = () => {};

  handleChange = () => {};

  handleAdd = () => {};

  onSelect = () => {};

  getHeight = (num) => {
    return `${num * 43}px`;
  };

  getFormatDate = (time) => {
    return moment(new Date(time)).utc().utcOffset(-6).format('YYYY-MM-DD HH:mm:ss');
  };

  render() {
    let { loadingAll, returnObj } = this.props;
    const { currentEnv } = this.props;
    loadingAll = loadingAll || false;
    returnObj = returnObj || [];
    const colorObj = {
      Deployed: 'first1',
      Staging: 'first2',
      UNKNOWN: 'first3',
    };
    return (
      <div>
        <div className="breadCrumb">
          <Breadcrumb>
            <Breadcrumb.Item>
              <Link to="/ApplicationList">Home</Link>
            </Breadcrumb.Item>
            <Breadcrumb.Item>Applications</Breadcrumb.Item>
          </Breadcrumb>
        </div>
        <PageContainer>
          <Spin spinning={loadingAll}>
            <div className="applist">
              <Form name="horizontal_login" layout="inline" onFinish={this.onFinish}>
                <Form.Item>
                  <Link to="/ApplicationList/CreateApplication">
                    <Button onClick={this.handleAdd} type="primary" style={{ marginBottom: 16 }}>
                      create
                    </Button>
                  </Link>
                </Form.Item>
              </Form>
            </div>
            <Row gutter={16}>
              {Array.isArray(returnObj) && returnObj.length ? (
                returnObj.map((item, index) => {
                  const { traits = [] } = item;
                  return (
                    <Col span={6} onClick={this.gotoDetail} key={index.toString()}>
                      <Link
                        to={{
                          pathname: '/ApplicationList/ApplicationListDetail',
                          state: { appName: item.name, envName: currentEnv },
                        }}
                      >
                        <Card
                          title={item.name}
                          bordered={false}
                          extra={this.getFormatDate(item.created)}
                        >
                          <div className="cardContent">
                            <div
                              className="box2"
                              style={{ height: this.getHeight(traits.length) }}
                            />
                            <div className="box1">
                              {traits.length ? (
                                <div className="box3" style={{ width: '30px' }} />
                              ) : (
                                ''
                              )}
                              <div
                                className={['hasPadding', colorObj[item.status] || 'first3'].join(
                                  ' ',
                                )}
                              >
                                <ApartmentOutlined style={{ marginRight: '4px' }} />
                                {item.workload}
                              </div>
                            </div>
                            {traits.map((item1, index1) => {
                              return (
                                <div className="box1" key={index1.toString()}>
                                  <div className="box3" style={{ width: '50px' }} />
                                  <div className="other hasPadding">
                                    <BranchesOutlined style={{ marginRight: '4px' }} />
                                    {item1}
                                  </div>
                                </div>
                              );
                            })}
                          </div>
                        </Card>
                      </Link>
                    </Col>
                  );
                })
              ) : (
                <div style={{ width: '100%', height: '80%' }}>
                  <Empty />
                </div>
              )}
            </Row>
          </Spin>
          <Modal
            title="Create Application"
            visible={this.state.visible}
            onOk={this.handleOk}
            onCancel={this.handleCancel}
            footer={[
              <Button key="submit" type="primary" onClick={this.handleOk}>
                Create
              </Button>,
            ]}
          >
            <Form {...layout} ref={this.formRef} name="control-ref" labelAlign="left">
              <Form.Item
                name="name"
                label="Name"
                rules={[
                  {
                    required: true,
                    message: 'Please input application name!',
                  },
                  {
                    pattern: /^[a-z0-9-_]+$/,
                    message:
                      'Name can only use digits(0-9),lowercase letters(a-z),and dashes(-),Underline.',
                  },
                ]}
              >
                <Input />
              </Form.Item>
              <Form.Item name="description" label="Description">
                <Input.TextArea />
              </Form.Item>
            </Form>
          </Modal>
        </PageContainer>
      </div>
    );
  }
}

export default TableList;
