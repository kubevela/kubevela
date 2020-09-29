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
 * POST /api/envs/:envName/apps/:appName/components/:compName/traits/ (attach a trait)
 */
export async function attachOneTraits({ envName, appName, compName, params }) {
  return request(`/api/envs/${envName}/apps/${appName}/components/${compName}/traits/`, {
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
 * DELETE /api/envs/:envName/apps/:appName/components/:compName/traits/:traitName (detach a trait)
 */
export async function deleteOneTrait({ envName, appName, compName, traitName }) {
  return request(
    `/api/envs/${envName}/apps/${appName}/components/${compName}/traits/${traitName}`,
    {
      method: 'delete',
    },
  );
}
