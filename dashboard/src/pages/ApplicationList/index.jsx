import React from "react";
import { PageContainer } from "@ant-design/pro-layout";
import { SearchOutlined, BranchesOutlined, ApartmentOutlined } from "@ant-design/icons";
import { Button, Card, Row, Col, Form, Select, DatePicker } from "antd";
import { connect } from 'dva';
import moment from "moment";
import "./index.less";
import { Link } from "umi";
const Option = Select.Option;

@connect(state => ({
  appList:state.testapi
}))

class TableList extends React.Component {
  constructor(props) {
    super(props);
  }

  onFinish = (values) => {
    console.log("Finish:", values);
    const data = moment(values.createTime).format('YYYY-MM-DD')
    console.log(data)
  };
  handleChange = () => {};
  handleAdd = () => {};
  onSelect = (selectedKeys, info) => {
    console.log("selected", selectedKeys, info);
  };
  getHeight = (num) =>{
    return (num * 52)+'px'
  };
  componentDidMount(){
    this.props.dispatch({
      type: 'testapi/getList',// testapi对应models层的命名空间namespace
      payload: {
        numCount: 1
      }
    })
  };
  render() {
    const { returnObj=[] } = this.props.appList
    return (
      <PageContainer>
        <div>
          <Form name="horizontal_login" layout="inline" onFinish={this.onFinish}>
            <Form.Item name="createTime">
              <DatePicker placeholder="createTime"/>
            </Form.Item>
            <Form.Item name="status">
              <Select
                placeholder="status"
                style={{ width: 120 }}
                onChange={this.handleChange}
              >
                <Option value="Status1">Status1</Option>
                <Option value="Status2">Status2</Option>
                <Option value="Status3">Status3</Option>
              </Select>
            </Form.Item>
            <Form.Item>
              <Button icon={<SearchOutlined />} htmlType="submit">Search</Button>
            </Form.Item>
            <Form.Item>
              <Link to="/ApplicationList/CreateApplication">
                <Button
                  onClick={this.handleAdd}
                  type="primary"
                  style={{ marginBottom: 16 }}
                >
                  create
                </Button>
              </Link>
            </Form.Item>
          </Form>
        </div>
        <Row gutter={16}>
          {
            Array.isArray(returnObj)
            ?
            returnObj.map((item,index)=>{
              return (
                <Col span={6} onClick={this.gotoDetail} key={index}>
                  <Link to="/ApplicationList/ApplicationListDetail">
                    <Card
                      title={item.name}
                      bordered={false}
                      extra={item.time}
                    >
                      <div className="cardContent">
                        <div className="box2" style={{ height: this.getHeight(item.traits.length) }}></div>
                        <div className="box1">
                          { item.traits.length ? <div className="box3" style={{ width: '40px' }}></div> : '' }
                          <div className={["hasPadding",item.status==1?"first2":"first1"].join(' ')} >
                            <ApartmentOutlined style={{ marginRight: '10px' }} />
                            {item.workLoad.name}
                          </div>
                        </div>
                        {
                          item.traits.map((item1)=>{
                            return (
                              <div className="box1" key={item1.id}>
                                <div className="box3" style={{ width: '80px' }}></div>
                                <div className="other hasPadding">
                                  <BranchesOutlined style={{ marginRight: '10px' }}/>
                                  {item1.name}
                                </div>
                              </div>
                            )
                          })
                        }
                      </div>
                    </Card>
                  </Link>
                </Col>
              )
            })
            :
            <div>暂无数据</div>
          }
        </Row>
      </PageContainer>
    );
  }
}

export default TableList;
