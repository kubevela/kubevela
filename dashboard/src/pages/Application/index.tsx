import React, { useCallback } from 'react';

import { Button, Card, message, Popconfirm, Space, Table } from 'antd';
import { Link, useModel, useRequest } from 'umi';

import { deleteApplication, getApplications } from '@/services/application';
import { PlusOutlined } from '@ant-design/icons';
import { PageContainer } from '@ant-design/pro-layout';
// @ts-ignore
import { Link as ReactLink } from 'react-router-dom';

export default () => {
  const { currentEnvironment } = useModel('useEnvironmentModel');
  const { data, loading, run: loadApps } = useRequest(
    async () => {
      if (currentEnvironment?.envName == null) {
        return { code: 200, data: [] } as API.VelaResponse<Array<API.Application>>;
      }
      return getApplications(currentEnvironment.envName);
    },
    {
      refreshDeps: [currentEnvironment],
    },
  );

  // delete application
  const remove = useCallback(
    async (appName: string) => {
      const envName = currentEnvironment?.envName;
      if (envName == null) {
        throw new Error('Unable to determine the current environment name.');
      }

      const response = await deleteApplication(envName, appName);
      loadApps();
      return response;
    },
    [currentEnvironment],
  );

  return (
    <PageContainer>
      <Card>
        <div style={{ marginBottom: '10px' }}>
          <Space>
            <Button type="primary" icon={<PlusOutlined />}>
              <ReactLink to="/applications/create"> Create</ReactLink>
            </Button>
          </Space>
        </div>
        <Table
          dataSource={data ?? []}
          rowKey={(record) => record.name}
          loading={loading ? { delay: 300 } : undefined}
          columns={[
            {
              title: 'Name',
              dataIndex: 'name',
              key: 'name',
              render: (text, record) => {
                return (
                  <Link
                    to={{
                      pathname: `${window.routerBase}applications/${record.name}`,
                    }}
                  >
                    {text}
                  </Link>
                );
              },
            },
            {
              title: 'Status',
              dataIndex: 'status',
              key: 'status',
              render: (text) => {
                return text;
              },
            },
            {
              title: 'Created Time',
              dataIndex: 'createdTime',
              key: 'createdTime',
              render: (text) => {
                return text;
              },
            },
            {
              title: 'Actions',
              dataIndex: 'Actions',
              key: 'Actions',
              render: (text, { name }) => {
                return (
                  <Space>
                    <Popconfirm
                      title="Are you sure to delete this application?"
                      onConfirm={() => {
                        remove(name).then(({ code, data: content }) => {
                          if (code === 200) {
                            message.success({
                              content,
                              key: 'remove',
                            });
                          } else {
                            message.error({
                              content,
                              key: 'remove',
                            });
                          }
                        });
                      }}
                    >
                      <Button type="link" size="small" danger>
                        Delete
                      </Button>
                    </Popconfirm>
                  </Space>
                );
              },
            },
          ]}
        />
      </Card>
    </PageContainer>
  );
};
