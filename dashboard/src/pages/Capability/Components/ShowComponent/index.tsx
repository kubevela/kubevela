import React, {useEffect, useState} from 'react';

import {Modal, Space, Table, Tag, Tooltip, Typography} from 'antd';

import {ShowParameters} from './types';

export default ({name, parameters}: ShowParameters) => {
  const [localVisible, setLocalVisible] = useState(false);

  const {Paragraph, Text} = Typography;


  const DEFAULT_DETAIL_STATE: ShowParameters = {
    name: '',
    parameters: [],
  };

  const [showTraits, setShowTraits] = useState<ShowParameters>({name,parameters})

  useEffect(() => {
    setLocalVisible(name !== "");
  }, [name, parameters]);

  function onCancel() {
    setShowTraits(DEFAULT_DETAIL_STATE);
  }

  return (
    <Modal
      forceRender
      visible={localVisible}
      onCancel={onCancel}
      footer={null}
      width={1000}
      maskClosable
      destroyOnClose
    >
      <h2>{`Type: ${showTraits.name}`}</h2>
      <Table columns={[
        {
          title: 'Name',
          dataIndex: 'name',
          key: 'name',
          width: 200,
          render: (text, row) => [
            <Paragraph
              copyable={{text: row.name}}
            >
              <Space size="small" align="center">
                {row.name}
                {!row.required ? undefined : (
                  <Tag color="geekblue">request</Tag>
                )}
              </Space>
            </Paragraph>
          ]
        },
        {
          title: 'Short',
          dataIndex: 'short',
          key: 'short',
          width: 100,
          responsive: ['md'],
          render: (text, row) => [
            <Space size="small" align="center">
              {!row.short ? undefined : (
                <Tag color="magenta">{text}</Tag>
              )}
            </Space>
          ]
        },
        {
          title: 'Usage',
          ellipsis: {
            showTitle: false,
          },
          dataIndex: 'usage',
          key: 'usage',
          render: address => (
            <Tooltip placement="topLeft" title={address}>
              {address}
            </Tooltip>
          ),
        },
        {
          title: 'Default',
          dataIndex: 'default',
          key: 'default',
          width: 200,
          responsive: ['md'],
          render: (text, row) => [
            <Space size="small" align="center">
              {!row.default ? undefined : (
                <Text code>{text}</Text>
              )}
            </Space>
          ]
        },
      ]}
             dataSource={showTraits.parameters}
             rowKey={record => record.name}
             pagination={{pageSize: 5, size: "small", hideOnSinglePage: true}}
      />
    </Modal>
  );
};
