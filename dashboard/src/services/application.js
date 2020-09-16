import request from '@/utils/request';
/*
 * 应用列表：/api/envs/default/apps/
 */
export async function getapplist({ url }) {
  return request(url);
}
/*
 * 创建应用：/api/envs/default/apps/
 */
export async function createApp({ params, url }) {
  return request(url, {
    method: 'POST',
    data: {
      ...params,
    },
    headers: {
      'Content-Type': 'application/json',
    },
  });
}
/*
 * GET /api/envs/:envName/apps/:appName (app 详情)
 */
export async function getAppDetail({ envName, appName }) {
  return request(`/api/envs/${envName}/apps/${appName}`);
}
/*
 * DELETE /api/envs/:envName/apps/:appName (删除 app)
 */
export async function deleteApp({ envName, appName }) {
  return request(`/api/envs/${envName}/apps/${appName}`, {
    method: 'delete',
  });
}
/*
 * PUT /api/envs/:envName/apps/:appName
 */
export async function updateApp({ envName, appName }) {
  return request(`/api/envs/${envName}/apps/${appName}`, {
    method: 'put',
  });
}
