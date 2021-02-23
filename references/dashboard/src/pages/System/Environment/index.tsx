import React, { useState } from 'react';

import { Button, Card, message, Popconfirm, Space, Table, Tag, Tooltip } from 'antd';
import { useModel, useRequest } from 'umi';

import { PlusOutlined } from '@ant-design/icons';
import { PageContainer } from '@ant-design/pro-layout';

import EditorComponent from './Components/EditorComponent';
import { EditMode, EditorState, Environment } from './types';

const DEFAULT_EDITOR_STATE: EditorState = {
  mode: 'create',
};

export default (): React.ReactNode => {
  const {
    loading,
    environments,
    switchCurrentEnvironment: switchEnvironment,
    deleteEnvironment,
  } = useModel('useEnvironmentModel');

  const [editor, setEditor] = useState<EditorState>(DEFAULT_EDITOR_STATE);

  const showEditor = ({
    mode,
    environment,
  }: {
    mode: EditMode;
    environment: Environment | null;
  }) => {
    setEditor({ mode, environment });
  };

  // delete env
  const { run: remove } = useRequest((e: { envName: string }) => deleteEnvironment(e.envName), {
    manual: true,
  });

  // switch current env
  const { run: switchCurrenrt } = useRequest(
    (e: { envName: string }) =>
      switchEnvironment(e.envName).then((env) =>
        message.success({
          content: `Set environment succeed, current environment is ${env?.envName}, namespace is ${env?.namespace}`,
          key: 'switchEnvironment',
        }),
      ),
    {
      manual: true,
    },
  );

  return (
    <PageContainer>
      <EditorComponent mode={editor.mode} environment={editor.environment} />
      <Card>
        <div style={{ marginBottom: '10px' }}>
          <Space>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() =>
                showEditor({ mode: 'create', environment: { envName: '', namespace: '' } })
              }
            >
              Create
            </Button>
          </Space>
        </div>
        <Table
          columns={[
            {
              title: 'Environment',
              dataIndex: 'envName',
              render: (text) => {
                if (text && text.length > 20) {
                  return <Tooltip title={text}>{text.substr(0, 20)}...</Tooltip>;
                }
                return text;
              },
            },
            {
              title: 'Namespace',
              dataIndex: 'namespace',
              ellipsis: { showTitle: false },
              render: (text) => <Tooltip title={text}>{text}</Tooltip>,
            },
            {
              title: 'Current',
              dataIndex: 'current',
              align: 'center',
              render: (text) => {
                return text === '*' ? <Tag color="success">Active</Tag> : undefined;
              },
            },
            {
              title: 'Actions',
              dataIndex: 'current',
              render: (current, { envName, namespace }) => {
                const active = current === '*';
                const defaultEnv = envName === 'default';
                return (
                  <Space>
                    <Button
                      size="small"
                      type="link"
                      disabled={active}
                      onClick={() => {
                        switchCurrenrt({ envName });
                      }}
                    >
                      Switch
                    </Button>
                    <Button
                      size="small"
                      type="link"
                      onClick={() =>
                        showEditor({
                          mode: 'update',
                          environment: {
                            envName,
                            namespace,
                          },
                        })
                      }
                    >
                      Update
                    </Button>
                    {defaultEnv ? undefined : (
                      <Popconfirm
                        title="Are you sure to delete this environment?"
                        onConfirm={() => {
                          remove({ envName }).then(() => {
                            message.success({
                              content: `Environment '${envName}' remove success!`,
                              key: 'remove',
                            });
                          });
                        }}
                      >
                        <Button type="link" size="small" danger>
                          Remove
                        </Button>
                      </Popconfirm>
                    )}
                  </Space>
                );
              },
            },
          ]}
          rowKey={(record) => record.envName}
          loading={loading ? { delay: 300 } : undefined}
          dataSource={environments ?? []}
        />
      </Card>
    </PageContainer>
  );
};
