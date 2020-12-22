import { request } from 'umi';

const BASE_PATH = '/api/traits';

/*
 * trait 列表: get /api/traits/
 */
export async function getTraits(): Promise<API.VelaResponse<API.Traits[]>> {
  return request(BASE_PATH);
}

