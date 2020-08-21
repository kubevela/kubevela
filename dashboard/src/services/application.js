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
    // body:JSON.stringify(params),
    data: {
      ...params,
    },
    headers: {
      'Content-Type': 'application/json',
    },
  });
}
