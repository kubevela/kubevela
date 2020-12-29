import React, { useState } from 'react';

import { Button, Card, Space, Tag, Typography } from 'antd';
import { useModel } from 'umi';
import { PageContainer } from '@ant-design/pro-layout';
import { FileWordTwoTone, SnippetsTwoTone } from '@ant-design/icons';
import ProList from '@ant-design/pro-list';
import { ShowParameters } from '@/pages/Capability/Components/ShowComponent/types';
import ShowComponent from '../Components/ShowComponent';

const DEFAULT_DETAIL_STATE: ShowParameters = {
  name: '',
  parameters: [],
};
const { Paragraph } = Typography;
export default (): React.ReactNode => {
  const { loading, workloadList } = useModel('useWorkloadsModel');

  const [showWorkload, setShowWorkload] = useState<ShowParameters>(DEFAULT_DETAIL_STATE);

  const showInfo = ({ workloads }: { workloads: ShowParameters }) => {
    setShowWorkload(workloads);
  };

  return (
    <PageContainer>
      <ShowComponent name={showWorkload.name} parameters={showWorkload.parameters} />
      <Card>
        <ProList<any>
          rowKey="name"
          headerTitle="Type"
          loading={loading ? { delay: 300 } : undefined}
          dataSource={workloadList ?? []}
          pagination={{
            defaultPageSize: 5,
            showSizeChanger: false,
          }}
          renderItem={(item) => item}
          split
          metas={{
            title: {
              dataIndex: 'name',
            },
            description: {
              render: (text, row) => {
                return (
                  <Paragraph ellipsis={{ rows: 1, expandable: true, symbol: 'more' }}>
                    {row.description}
                  </Paragraph>
                );
              },
            },
            subTitle: {
              render: (text, row) => {
                return (
                  <Space size={0}>
                    {!row.required ? undefined : <Tag color="#5BD8A6">Required</Tag>}
                  </Space>
                );
              },
            },
            actions: {
              render: (text, row) => [
                <Button
                  size="small"
                  type="link"
                  icon={<SnippetsTwoTone />}
                  onClick={() => showInfo({ workloads: row })}
                >
                  details
                </Button>,
                <Button
                  size="small"
                  type="link"
                  icon={<FileWordTwoTone />}
                  href={`https://kubevela.io/#/en/developers/references/workload-types/${row.name}`}
                  target="view_window"
                >
                  reference
                </Button>,
              ],
            },
          }}
        />
      </Card>
    </PageContainer>
  );
};
