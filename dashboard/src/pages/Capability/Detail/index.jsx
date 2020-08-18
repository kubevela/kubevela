import React from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Space, Modal, Button, Row, Col } from 'antd';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import './index.less';

const { confirm } = Modal;

class TableList extends React.PureComponent {
  gotoOtherPage = () => {
    // window.location.href = 'https://github.com/oam-dev/catalog/blob/master/workloads/cloneset/README.md';
    window.open('https://github.com/oam-dev/catalog/blob/master/workloads/cloneset/README.md');
  };

  installSignle = (e) => {
    e.stopPropagation();
  };

  showDeleteConfirm = () => {
    confirm({
      title: `Are you sure delete this Task?`,
      icon: <ExclamationCircleOutlined />,
      width: 500,
      content: (
        <div>
          <p style={{ margin: '0px' }}>您本次移除 capability center，将会删除的应用列表：</p>
          <Space>
            <span>abc</span>
            <span>abc</span>
            <span>abc</span>
            <span>abc</span>
          </Space>
          <p style={{ margin: '0px' }}>确认后，移除该 capability center，并且删除相应的应用？</p>
        </div>
      ),
      okText: 'Yes',
      okType: 'danger',
      cancelText: 'No',
      onOk() {
        // console.log('OK');
      },
      onCancel() {
        // console.log('Cancel');
      },
    });
  };

  render() {
    return (
      <PageContainer>
        <div style={{ marginBottom: '16px' }}>
          <Space>
            <Button type="primary">Install all</Button>
            <Button type="default" onClick={this.showDeleteConfirm}>
              Remove
            </Button>
          </Space>
        </div>
        <div>
          <h3>Workloads</h3>
          <Row>
            <Col span="4">
              <div className="itemBox" onClick={this.gotoOtherPage}>
                <img
                  src="https://ss0.bdstatic.com/70cFvHSh_Q1YnxGkpoWK1HF6hhy/it/u=1109866916,1852667152&fm=26&gp=0.jpg"
                  alt="workload"
                />
                <p>CloneSet v1.4.7</p>
                <Button onClick={this.installSignle}>install</Button>
              </div>
            </Col>
          </Row>
        </div>
        <div>
          <h3>Traits</h3>
          <Row>
            <Col span="4">
              <div className="itemBox" onClick={this.gotoOtherPage}>
                <img
                  src="https://ss0.bdstatic.com/70cFvHSh_Q1YnxGkpoWK1HF6hhy/it/u=1109866916,1852667152&fm=26&gp=0.jpg"
                  alt="workload"
                />
                <p>Knative Serving v1.4.7</p>
                <Button onClick={this.installSignle}>install</Button>
              </div>
            </Col>
          </Row>
        </div>
      </PageContainer>
    );
  }
}

export default TableList;
