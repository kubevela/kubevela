import React, {useState} from 'react';

import {Button, Card, Space, Tag} from 'antd';
import {useModel} from 'umi';
import {PageContainer} from '@ant-design/pro-layout';
import {Workloads} from './types';
import {SearchOutlined} from '@ant-design/icons';
import ShowComponent from './Components/ShowComponent'
import ProList from "@ant-design/pro-list";

const DEFAULT_DETAIL_STATE: Workloads = {
  name: '',
  parameters: [],
};

export default (): React.ReactNode => {
  const {
    loading,
    workloadList,
  } = useModel('useWorkloadsModel');

  const [showWorkload, setShowWorkload] = useState<Workloads>(DEFAULT_DETAIL_STATE)

  const showInfo = ({
                      workloads,
                    }: {
    workloads: Workloads;
  }) => {
    setShowWorkload(workloads);
  };

  return (
    <PageContainer>
      <ShowComponent name={showWorkload.name} parameters={showWorkload.parameters}/>
      <Card>
        <ProList<any>
          rowKey="name"
          headerTitle="Type"
          loading={loading ? {delay: 300} : undefined}
          dataSource={workloadList ?? []}
          // showActions="hover"
          metas={{
            title: {
              dataIndex: 'name',
            },
            subTitle: {
              render: (text, row) => {
                return (
                  <Space size={0}>
                    {!row.required ? undefined : (
                      <Tag color="#5BD8A6">Required</Tag>
                    )}
                  </Space>
                );
              },
            },
            actions: {
              render: (text, row) => [
                <Button
                  size="small"
                  type="link"
                  onClick={() => showInfo({workloads: row})}
                >
                  detail
                </Button>,
                <Button
                  size="small"
                  type="link"
                  icon={<SearchOutlined/>}
                  href={`https://kubevela.io/#/en/developers/references/workload-types/${row.name}`}
                  target="view_window"
                >
                  reference
                </Button>
              ],
            },
          }}
        />
      </Card>
    </PageContainer>
  );
};
