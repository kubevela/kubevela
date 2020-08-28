import React from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { SearchOutlined, BranchesOutlined, ApartmentOutlined } from '@ant-design/icons';
import { Button, Card, Row, Col, Form, Select, DatePicker, Spin, Empty } from 'antd';
import { connect } from 'dva';
import moment from 'moment';
import './index.less';
import { Link } from 'umi';

const { Option } = Select;

@connect(({ loading, applist, globalData }) => ({
  loadingAll: loading.models.applist,
  // 当applist这个models有数据请求行为的时候，loading为true，没有请求的时候为false
  // loadingList: loading.effects['applist/getList'],
  // 当applist的effects中的getList有异步请求行为时为true，没有请求行为时为false
  returnObj: applist.returnObj,
  currentEnv: globalData.currentEnv,
}))
class TableList extends React.Component {
  constructor(props) {
    super(props);
    this.state = {};
  }

  componentDidMount() {
    const { currentEnv } = this.props;
    if (currentEnv) {
      this.props.dispatch({
        type: 'applist/getList', // applist对应models层的命名空间namespace
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
      type: 'applist/getList', // applist对应models层的命名空间namespace
      payload: {
        url: `/api/envs/${nextProps.currentEnv}/apps/`,
      },
    });
    return true;
    // return true;
  }

  onFinish = () => {
    // const data = moment(values.createTime).format('YYYY-MM-DD')
  };

  handleChange = () => {};

  handleAdd = () => {};

  onSelect = () => {
    // console.log("selected", selectedKeys, info);
  };

  getHeight = (num) => {
    return `${num * 52}px`;
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
      True: 'first1',
      False: 'first2',
      UNKNOWN: 'first3',
    };
    return (
      <PageContainer>
        <Spin spinning={loadingAll}>
          <div>
            <Form name="horizontal_login" layout="inline" onFinish={this.onFinish}>
              <Form.Item name="createTime">
                <DatePicker placeholder="createTime" />
              </Form.Item>
              <Form.Item name="status">
                <Select
                  placeholder="status"
                  style={{ width: 120 }}
                  onChange={this.handleChange}
                  allowClear
                >
                  <Option value="True">True</Option>
                  <Option value="False">False</Option>
                  <Option value="UNKNOWN">UNKNOWN</Option>
                </Select>
              </Form.Item>
              <Form.Item>
                <Button icon={<SearchOutlined />} htmlType="submit">
                  Search
                </Button>
              </Form.Item>
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
                          <div className="box2" style={{ height: this.getHeight(traits.length) }} />
                          <div className="box1">
                            {traits.length ? (
                              <div className="box3" style={{ width: '40px' }} />
                            ) : (
                              ''
                            )}
                            <div className={['hasPadding', colorObj[item.status]].join(' ')}>
                              <ApartmentOutlined style={{ marginRight: '10px' }} />
                              {item.workload}
                            </div>
                          </div>
                          {traits.map((item1, index1) => {
                            return (
                              <div className="box1" key={index1.toString()}>
                                <div className="box3" style={{ width: '80px' }} />
                                <div className="other hasPadding">
                                  <BranchesOutlined style={{ marginRight: '10px' }} />
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
      </PageContainer>
    );
  }
}

export default TableList;
