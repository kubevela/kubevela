import React, {useEffect, useState} from 'react';

import {Modal, Space, Tag} from 'antd';
import ProList from '@ant-design/pro-list';

import {Workloads} from '../../types';

export default ({name, parameters}: Workloads) => {
  const [localVisible, setLocalVisible] = useState(false);

  useEffect(() => {
    setLocalVisible(name !== "");
  }, [name]);

  return (
    <Modal
      forceRender
      visible={localVisible}
      onCancel={() => setLocalVisible(false)}
      footer={null}
      width={1000}
      maskClosable
    >
      <ProList<any>
        rowKey="name"
        headerTitle={`Type: ${name}`}
        dataSource={parameters ?? []}
        showActions="hover"
        metas={{
          title: {
            dataIndex: 'name',
          },
          description: {
            dataIndex: 'usage',
          },
          subTitle: {
            render: (text, row) => {
              return (
                <Space size={0}>
                  {!row.required ? undefined : (
                    <Tag color="#5BD8A6">Request</Tag>
                  )}
                </Space>
              );
            },
          },
          actions: {
            render: (text, row) => [
              <a href={row.html_url} target="_blank" rel="noopener noreferrer" key="link">
                {text}
              </a>,
              <a href={row.html_url} target="_blank" rel="noopener noreferrer" key="warning">
                报警
              </a>,
              <a href={row.html_url} target="_blank" rel="noopener noreferrer" key="view">
                查看
              </a>,
            ],
          },
        }}
      />
    </Modal>
  );
};
