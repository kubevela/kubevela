import request from '@/utils/request';
/*
 * 获取单个 trait: GET /api/traits/:traitName
 */
export async function getTraitByName({ traitName }) {
  return request(`/api/traits/${traitName}`);
}
/*
 * traits 列表：GET  /api/traits/
 */
export async function getTraits() {
  return request('/api/traits/');
}
/*
 * POST /api/envs/:envName/apps/:appName/traits/ (attach 单个 trait)
 */
export async function attachOneTraits({ envName, appName, params }) {
  return request(`/api/envs/${envName}/apps/${appName}/traits/`, {
    method: 'post',
    data: {
      ...params,
    },
    headers: {
      'Content-Type': 'application/json',
    },
  });
}
/*
 * DELETE /api/envs/:envName/apps/:appName/traits/:traitName (detach 单个 trait)
 */
export async function deleteOneTrait({ envName, appName, traitName }) {
  return request(`/api/envs/${envName}/apps/${appName}/traits/${traitName}`, {
    method: 'delete',
  });
}
