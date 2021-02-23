import React, { useState } from 'react';

import { Button, Card, Typography } from 'antd';
import { useModel } from 'umi';

import { ShowParameters } from '@/pages/Capability/Components/ShowComponent/types';
import { FileWordTwoTone, SnippetsTwoTone } from '@ant-design/icons';
import { PageContainer } from '@ant-design/pro-layout';
import ProList from '@ant-design/pro-list';

import ShowComponent from '../Components/ShowComponent';

const DEFAULT_DETAIL_STATE: ShowParameters = {
  name: '',
  parameters: [],
};
const { Paragraph } = Typography;
export default (): React.ReactNode => {
  const { loading, workloadList } = useModel('useWorkloadsModel');

  const [showWorkload, setShowWorkload] = useState<ShowParameters>(DEFAULT_DETAIL_STATE);

  const showInfo = ({ parameters }: { parameters: ShowParameters }) => {
    setShowWorkload(parameters);
  };

  return (
    <PageContainer>
      <ShowComponent name={showWorkload.name} parameters={showWorkload.parameters} />
      <Card>
        <ProList<API.Workloads>
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
              render: (_, row) => {
                return (
                  <Paragraph ellipsis={{ rows: 1, expandable: true, symbol: 'more' }}>
                    {row.description}
                  </Paragraph>
                );
              },
            },
            actions: {
              render: (_, { name, parameters }) => {
                const actions = [];

                if (parameters != null && parameters.length > 0) {
                  actions.push(
                    <Button
                      size="small"
                      type="link"
                      icon={<SnippetsTwoTone />}
                      onClick={() =>
                        showInfo({
                          parameters: {
                            name,
                            parameters,
                          },
                        })
                      }
                    >
                      details
                    </Button>,
                  );
                }

                actions.push(
                  <Button
                    size="small"
                    type="link"
                    icon={<FileWordTwoTone />}
                    href={`https://kubevela.io/#/en/developers/references/workload-types/${name}`}
                    target="view_window"
                  >
                    reference
                  </Button>,
                );

                return actions;
              },
            },
          }}
        />
      </Card>
    </PageContainer>
  );
};
