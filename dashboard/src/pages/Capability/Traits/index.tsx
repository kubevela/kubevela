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
const { Paragraph, Text } = Typography;
export default (): React.ReactNode => {
  const { loading, traitsList } = useModel('useTraitsModel');

  const [showTraits, setShowTraits] = useState<ShowParameters>(DEFAULT_DETAIL_STATE);

  const showInfo = ({ traits }: { traits: ShowParameters }) => {
    setShowTraits(traits);
  };

  return (
    <PageContainer>
      <ShowComponent name={showTraits.name} parameters={showTraits.parameters} />
      <Card>
        <ProList<any>
          rowKey={(record) => record.name}
          headerTitle="Type"
          pagination={{
            defaultPageSize: 5,
            showSizeChanger: false,
          }}
          loading={loading ? { delay: 300 } : undefined}
          dataSource={traitsList ?? []}
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
                    <Tag color="green">{row.crdName}</Tag>
                  </Space>
                );
              },
            },
            content: {
              render: (text, row) => {
                return (
                  <Space size={0}>
                    <Text strong>applies&nbsp;to:&nbsp;</Text>
                    {row.appliesTo.map((item: string) => (
                      <Tag color="processing">{item}</Tag>
                    ))}
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
                  onClick={() => showInfo({ traits: row })}
                >
                  details
                </Button>,
                <Button
                  size="small"
                  type="link"
                  icon={<FileWordTwoTone />}
                  href={`https://kubevela.io/#/en/developers/references/traits/${row.name}`}
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
