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
