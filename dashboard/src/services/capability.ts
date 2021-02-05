import { request } from 'umi';


/*
 * workload type list: get /api/workloads/
 */
export async function getWorkloads(): Promise<API.VelaResponse<API.Workloads[]>> {
  return request( '/api/workloads');
}


/*
 * trait list: get /api/traits/
 */
export async function getTraits(): Promise<API.VelaResponse<API.Traits[]>> {
  return request('/api/traits');
}

export async function getCapabilityOpenAPISchema(
  capabilityName: string,
): Promise<API.VelaResponse<string>> {
  return request(`/api/definitions/${capabilityName}`, { method: 'get' });
}
