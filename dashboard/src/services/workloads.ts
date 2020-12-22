import { request } from 'umi';

const BASE_PATH = '/api/workloads';

/*
 * workload 列表: get /api/workloads/
 */
export async function getWorkloads(): Promise<API.VelaResponse<API.Workloads[]>> {
  return request(BASE_PATH);
}

